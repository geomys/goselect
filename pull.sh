#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
	echo "Usage: $0 <tag>"
	exit 1
fi

TAG="$1"
ROOT="$(cd "$(dirname "$0")" && pwd)"
WORK="$(mktemp -d)"

cleanup() {
	rm -rf "$WORK"
}
trap cleanup EXIT

command -v git >/dev/null
command -v git-filter-repo >/dev/null

(cd "$ROOT" && go build -o "$WORK/rewrite" ./internal/upstream/rewrite)
export GOUP_REWRITE="$WORK/rewrite"

if [ -d "$HOME/go/.git" ]; then
	REFERENCE=(--reference "$HOME/go" --dissociate)
else
	REFERENCE=()
fi

git -c advice.detachedHead=false clone --no-checkout --single-branch "${REFERENCE[@]}" \
	-b "$TAG" https://go.googlesource.com/go.git "$WORK/go"

# goup requires the version of golang.org/x/mod vendored in cmd.
XMOD="$(git -C "$WORK/go" show HEAD:src/cmd/go.mod | awk '$1 == "golang.org/x/mod" { print $2 }')"

# git filter-repo fails to parse multi-line values in the global git config,
# which the temporary clone doesn't need anyway.
export GIT_CONFIG_GLOBAL=/dev/null

# Keep only the paths goup is built from, at their goup locations.
git -C "$WORK/go" filter-repo --force \
	--paths-from-file "$ROOT/internal/upstream/paths.txt" \
	--prune-empty always \
	--prune-degenerate always \
	--tag-callback 'tag.skip()'

# Drop the unused files of kept directories, and pass every version of every
# Go file through internal/upstream/rewrite, caching results by name and blob.
read -r -d '' CALLBACK <<'PY' || true
import os, subprocess
d = value.data
if 'proc' not in d:
    d['proc'] = subprocess.Popen([os.environ['GOUP_REWRITE'], '-server'],
                                 stdin=subprocess.PIPE, stdout=subprocess.PIPE)
    d['cache'] = {}
if not filename.endswith(b'.go'):
    return (filename, mode, blob_id)
key = (filename, blob_id)
if key not in d['cache']:
    contents = value.get_contents_by_identifier(blob_id)
    p = d['proc']
    p.stdin.write(b'%d %d\n' % (len(filename), len(contents)) + filename + contents)
    p.stdin.flush()
    out = p.stdout.read(int(p.stdout.readline()))
    d['cache'][key] = blob_id if out == contents else value.insert_file_with_contents(out)
return (filename, mode, d['cache'][key])
PY
git -C "$WORK/go" filter-repo --force --invert-paths \
	--paths-from-file "$ROOT/internal/upstream/exclude.txt" \
	--prune-empty always \
	--prune-degenerate always \
	--file-info-callback "$CALLBACK"

unset GIT_CONFIG_GLOBAL
git -C "$ROOT" fetch "$WORK/go"
git -C "$ROOT" update-ref "refs/heads/upstream/$TAG" FETCH_HEAD

echo
echo "Fetched rewritten upstream history up to $TAG. Merge with:"
echo -e "\tjj new main upstream/$TAG"
echo "and update golang.org/x/mod to the version vendored in cmd with:"
echo -e "\tgo mod edit -require=golang.org/x/mod@$XMOD"
