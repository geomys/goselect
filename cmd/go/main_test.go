package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/mod/sumdb"
	"golang.org/x/mod/sumdb/dirhash"
	"golang.org/x/mod/sumdb/note"
)

func TestMain(m *testing.M) {
	if os.Getenv("GOUP_TEST_MAIN") == "1" {
		main()
		panic("unreachable")
	}
	os.Exit(m.Run())
}

// testToolchains are the fake toolchains served by the test proxy.
// The latest release is go1.27.3, so the default toolchain is go1.26.10.
var testToolchains = []string{"go1.16", "go1.21.0", "go1.26.9", "go1.26.10", "go1.27.0", "go1.27.3"}

// testProxy is a module proxy serving fake golang.org/toolchain modules for
// the current platform, which also proxies a checksum database for them.
type testProxy struct {
	srv  *httptest.Server
	vkey string // GOSUMDB value

	mu       sync.Mutex
	listFail bool            // fail @v/list requests
	tampered map[string]bool // toolchains whose served zip doesn't match the sumdb
	requests map[string]int  // count of requests by URL path
}

const sumdbName = "sum.test.example"

func toolchainModVersion(tv string) string {
	return "v0.0.1-" + tv + "." + runtime.GOOS + "-" + runtime.GOARCH
}

// fakeToolchainZip returns a module zip for a toolchain whose bin/go reports
// its name, arguments, and environment.
func fakeToolchainZip(t *testing.T, tv string, extra string) []byte {
	t.Helper()
	prefix := "golang.org/toolchain@" + toolchainModVersion(tv) + "/"
	files := []struct{ name, data string }{
		{"go.mod", "module golang.org/toolchain\n"},
		{"bin/go", "#!/bin/sh\necho \"fake " + tv + " args=[$*] target=[$GOTOOLCHAIN_INTERNAL_SWITCH_VERSION] goroot=[$GOROOT] pwd=[$(pwd)]\"\n" + extra},
		{"bin/gofmt", "#!/bin/sh\n"},
		{"lib/wasm/go_js_wasm_exec", "#!/bin/sh\n"},
		{"pkg/tool/fake", "#!/bin/sh\n"},
		{"src/_go.mod", "module std\n"},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(prefix + f.name)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, f.data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newTestProxy(t *testing.T) *testProxy {
	t.Helper()
	p := &testProxy{tampered: map[string]bool{}, requests: map[string]int{}}

	zips := map[string][]byte{} // module version -> zip
	sums := map[string]string{} // module version -> go.sum lines
	dir := t.TempDir()
	goModHash, err := dirhash.Hash1([]string{"go.mod"}, func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("module golang.org/toolchain\n")), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tv := range testToolchains {
		v := toolchainModVersion(tv)
		zips[v] = fakeToolchainZip(t, tv, "")
		file := filepath.Join(dir, v+".zip")
		if err := os.WriteFile(file, zips[v], 0o666); err != nil {
			t.Fatal(err)
		}
		h, err := dirhash.HashZip(file, dirhash.DefaultHash)
		if err != nil {
			t.Fatal(err)
		}
		sums[v] = fmt.Sprintf("golang.org/toolchain %s %s\ngolang.org/toolchain %s/go.mod %s\n", v, h, v, goModHash)
	}

	skey, vkey, err := note.GenerateKey(rand.Reader, sumdbName)
	if err != nil {
		t.Fatal(err)
	}
	p.vkey = vkey
	db := sumdb.NewServer(sumdb.NewTestServer(skey, func(path, vers string) ([]byte, error) {
		if s, ok := sums[vers]; ok && path == "golang.org/toolchain" {
			return []byte(s), nil
		}
		return nil, fmt.Errorf("unknown module %s@%s", path, vers)
	}))

	const modPrefix = "/golang.org/toolchain/@v/"
	sumdbPrefix := "/sumdb/" + sumdbName
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.requests[r.URL.Path]++
		listFail := p.listFail
		tampered := p.tampered
		p.mu.Unlock()

		switch path := r.URL.Path; {
		case path == sumdbPrefix+"/supported":
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(path, sumdbPrefix+"/"):
			http.StripPrefix(sumdbPrefix, db).ServeHTTP(w, r)
		case path == modPrefix+"list":
			if listFail {
				http.Error(w, "list unavailable", http.StatusInternalServerError)
				return
			}
			for _, tv := range testToolchains {
				fmt.Fprintln(w, toolchainModVersion(tv))
			}
		case strings.HasPrefix(path, modPrefix):
			// The version itself contains dots, so cut at the last one.
			name := strings.TrimPrefix(path, modPrefix)
			i := strings.LastIndex(name, ".")
			if i < 0 {
				http.NotFound(w, r)
				return
			}
			v, ext := name[:i], name[i+1:]
			data, ok := zips[v]
			if !ok {
				http.NotFound(w, r)
				return
			}
			switch ext {
			case "info":
				fmt.Fprintf(w, `{"Version":%q}`, v)
			case "mod":
				io.WriteString(w, "module golang.org/toolchain\n")
			case "zip":
				for tv := range tampered {
					if toolchainModVersion(tv) == v {
						data = fakeToolchainZip(t, tv, "echo evil\n")
					}
				}
				w.Write(data)
			default:
				http.NotFound(w, r)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *testProxy) count(path string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests[path]
}

func (p *testProxy) setListFail(fail bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listFail = fail
}

func (p *testProxy) tamper(tv string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tampered[tv] = true
}

// testEnv is an isolated environment to run goup in.
type testEnv struct {
	t     *testing.T
	root  string
	proxy *testProxy
	env   []string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	switch runtime.GOOS {
	case "windows", "plan9", "js", "wasip1", "ios", "android":
		t.Skipf("fake toolchains are shell scripts, unsupported on %s", runtime.GOOS)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The module cache is read-only, so make it writable before the
	// testing package removes the temporary directory.
	t.Cleanup(func() {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				os.Chmod(path, 0o777)
			}
			return nil
		})
	})
	e := &testEnv{t: t, root: root, proxy: newTestProxy(t)}
	for _, d := range []string{"home", "gopath", "bin", "work"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o777); err != nil {
			t.Fatal(err)
		}
	}
	e.env = []string{
		"GOUP_TEST_MAIN=1",
		"HOME=" + filepath.Join(root, "home"),
		"PATH=" + filepath.Join(root, "bin"),
		"GOPATH=" + filepath.Join(root, "gopath"),
		"GOENV=" + filepath.Join(root, "goenv"),
		"GOPROXY=" + e.proxy.srv.URL,
		"GOSUMDB=" + e.proxy.vkey,
	}
	return e
}

// write writes a file under the work directory.
func (e *testEnv) write(name, data string) {
	e.t.Helper()
	name = filepath.Join(e.root, "work", name)
	if err := os.MkdirAll(filepath.Dir(name), 0o777); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(data), 0o666); err != nil {
		e.t.Fatal(err)
	}
}

// pathToolchain installs a fake toolchain named tv in PATH.
func (e *testEnv) pathToolchain(tv string) {
	e.t.Helper()
	script := "#!/bin/sh\necho \"path " + tv + " args=[$*]\"\n"
	if err := os.WriteFile(filepath.Join(e.root, "bin", tv), []byte(script), 0o777); err != nil {
		e.t.Fatal(err)
	}
}

// run runs goup in the subdirectory dir of the work directory, with the
// extra environment variables env, and returns its combined output.
func (e *testEnv) run(dir string, env []string, args ...string) (string, error) {
	e.t.Helper()
	exe, err := os.Executable()
	if err != nil {
		e.t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Join(e.root, "work", dir)
	cmd.Env = append(slices.Clip(e.env), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ok runs goup like run and fails the test if it fails or if its output
// doesn't contain all of want.
func (e *testEnv) ok(dir string, env []string, args []string, want ...string) string {
	e.t.Helper()
	out, err := e.run(dir, env, args...)
	if err != nil {
		e.t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			e.t.Errorf("go %s: output doesn't contain %q:\n%s", strings.Join(args, " "), w, out)
		}
	}
	return out
}

// fail runs goup like run and fails the test if it succeeds or if its
// output doesn't contain all of want.
func (e *testEnv) fail(dir string, env []string, args []string, want ...string) string {
	e.t.Helper()
	out, err := e.run(dir, env, args...)
	if err == nil {
		e.t.Fatalf("go %s: unexpected success\n%s", strings.Join(args, " "), out)
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			e.t.Errorf("go %s: output doesn't contain %q:\n%s", strings.Join(args, " "), w, out)
		}
	}
	return out
}

func args(s string) []string { return strings.Fields(s) }

func env(kv ...string) []string { return kv }

func TestGoLine(t *testing.T) {
	e := newTestEnv(t)
	e.write("m/go.mod", "module m\n\ngo 1.21\n")
	out := e.ok("m", nil, args("version -x"),
		"go: downloading go1.21.0 ("+runtime.GOOS+"/"+runtime.GOARCH+")",
		"fake go1.21.0 args=[version -x] target=[go1.21.0]")
	modDir := filepath.Join(e.root, "gopath", "pkg", "mod", "golang.org", "toolchain@"+toolchainModVersion("go1.21.0"))
	if !strings.Contains(out, "goroot=["+modDir+"]") {
		t.Errorf("GOROOT is not the module cache directory %s:\n%s", modDir, out)
	}
	if _, err := os.Stat(filepath.Join(modDir, "src", "go.mod")); err != nil {
		t.Errorf("src/_go.mod was not copied to src/go.mod: %v", err)
	}

	// The second run uses the module cache.
	zip := "/golang.org/toolchain/@v/" + toolchainModVersion("go1.21.0") + ".zip"
	if n := e.proxy.count(zip); n != 1 {
		t.Errorf("zip downloaded %d times, want 1", n)
	}
	out = e.ok("m", nil, args("version"), "fake go1.21.0")
	if strings.Contains(out, "downloading") {
		t.Errorf("toolchain downloaded again:\n%s", out)
	}
	if n := e.proxy.count(zip); n != 1 {
		t.Errorf("zip downloaded %d times, want 1", n)
	}
}

func TestToolchainLine(t *testing.T) {
	e := newTestEnv(t)
	e.write("m/go.mod", "module m\n\ngo 1.21\n\ntoolchain go1.27.3\n")
	e.ok("m", nil, args("build ./..."), "fake go1.27.3 args=[build ./...]")
}

func TestGoWork(t *testing.T) {
	e := newTestEnv(t)
	e.write("w/go.work", "go 1.26.9\n\nuse ./m\n")
	e.write("w/m/go.mod", "module m\n\ngo 1.21\n")
	e.ok("w/m", nil, args("list"), "fake go1.26.9")
	e.ok("w/m", env("GOWORK=off"), args("list"), "fake go1.21.0")
}

func TestGOTOOLCHAINVersion(t *testing.T) {
	e := newTestEnv(t)
	e.write("m/go.mod", "module m\n\ngo 1.27.3\n")
	e.write("old/go.mod", "module m\n\ngo 1.21\n")
	// An exact version doesn't allow upgrades.
	e.ok("m", env("GOTOOLCHAIN=go1.27.0"), args("version"), "fake go1.27.0")
	// A minimum version allows upgrades, and is the floor otherwise.
	e.ok("m", env("GOTOOLCHAIN=go1.26.9+auto"), args("version"), "fake go1.27.3")
	e.ok("old", env("GOTOOLCHAIN=go1.27.0+auto"), args("version"), "fake go1.27.0")
	// Outside a module, the minimum version runs.
	e.ok("", env("GOTOOLCHAIN=go1.26.9+auto"), args("version"), "fake go1.26.9")
	// local+auto means auto.
	e.ok("m", env("GOTOOLCHAIN=local+auto"), args("version"), "fake go1.27.3")
}

func TestPathMode(t *testing.T) {
	e := newTestEnv(t)
	e.write("m/go.mod", "module m\n\ngo 1.27.3\n")
	e.fail("m", env("GOTOOLCHAIN=path"), args("version"), `cannot find "go1.27.3" in PATH`)
	e.pathToolchain("go1.27.3")
	e.ok("m", env("GOTOOLCHAIN=path"), args("version"), "path go1.27.3 args=[version]")
	// Toolchains in PATH are preferred in auto mode too.
	e.ok("m", nil, args("version"), "path go1.27.3")
	if n := e.proxy.count("/golang.org/toolchain/@v/" + toolchainModVersion("go1.27.3") + ".zip"); n != 0 {
		t.Errorf("toolchain in PATH was downloaded")
	}
}

func TestPathModeNotExecutable(t *testing.T) {
	e := newTestEnv(t)
	e.pathToolchain("go1.27.3")
	// Symlinks have permission bits of their own, which don't matter.
	bin := filepath.Join(e.root, "bin")
	if err := os.Symlink(filepath.Join(e.root, "missing"), filepath.Join(bin, "go1.26.10")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.root, "go1.26.9"), []byte("#!/bin/sh\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(e.root, "go1.26.9"), filepath.Join(bin, "go1.26.9")); err != nil {
		t.Fatal(err)
	}
	e.ok("", env("GOTOOLCHAIN=path", "GODEBUG=toolchaintrace=1"), args("version"),
		"go: default toolchain set to go1.27.3 from the toolchains in PATH", "path go1.27.3")
}

func TestChdir(t *testing.T) {
	e := newTestEnv(t)
	e.write("a/go.mod", "module a\n\ngo 1.26.9\n")
	e.write("b/go.mod", "module b\n\ngo 1.27.0\n")
	e.ok("a", nil, args("-C ../b version"), "fake go1.27.0 args=[version]", "pwd=["+filepath.Join(e.root, "work", "b")+"]")
	e.ok("a", nil, args("build -C ../b ./..."), "fake go1.27.0 args=[build ./...]")
	e.ok("a", nil, args("mod tidy -C ../b"), "fake go1.27.0 args=[mod tidy]")
	e.ok("a", nil, args("mod -C ../b tidy"), "fake go1.27.0 args=[mod tidy]")
	// -C after a non-flag argument belongs to the selected toolchain.
	e.ok("a", nil, args("run main.go -C ../b"), "fake go1.26.9 args=[run main.go -C ../b]")
}

func TestToolchainTrace(t *testing.T) {
	e := newTestEnv(t)
	e.write("m/go.mod", "module m\n\ngo 1.21\n\ntoolchain go1.27.3\n")
	e.ok("m", env("GODEBUG=toolchaintrace=1"), args("version"),
		"go: upgrading toolchain to go1.27.3 (required by toolchain line in go.mod; upgrade allowed by GOTOOLCHAIN=auto)",
		"go: using go1.27.3 toolchain from cache located at ",
		"fake go1.27.3")
}

func TestSwitchLoop(t *testing.T) {
	e := newTestEnv(t)
	e.write("m/go.mod", "module m\n\ngo 1.21\n")
	e.fail("m", env("GOTOOLCHAIN_INTERNAL_SWITCH_COUNT=100"), args("version"), "too many toolchain switches")
	e.fail("m", env("GOTOOLCHAIN_INTERNAL_SWITCH_VERSION=go1.21.0"), args("version"),
		"goup invoked to provide toolchain go1.21.0")
}

func TestChecksumDatabase(t *testing.T) {
	e := newTestEnv(t)
	e.write("m/go.mod", "module m\n\ngo 1.27.0\n")
	e.proxy.tamper("go1.27.0")
	e.fail("m", nil, args("version"), "checksum mismatch", "SECURITY ERROR")
	// Settings that upstream honors for other modules don't apply.
	e.fail("m", env("GOSUMDB=off"), args("version"), "checksum database disabled by GOSUMDB=off")
	e.fail("m", env("GONOSUMDB=*", "GOPRIVATE=*"), args("version"), "checksum mismatch")
	e.fail("m", env("GOPROXY="+e.proxy.srv.URL+",off"), args("version"), "checksum mismatch")
	e.fail("m", env("GIT_HTTP_USER_AGENT=proxy.golang.org"), args("version"), "checksum mismatch")
}

func TestPrivateSettingsIgnored(t *testing.T) {
	e := newTestEnv(t)
	e.write("m/go.mod", "module m\n\ngo 1.27.0\n")
	e.ok("m", env("GOPRIVATE=*", "GONOPROXY=*", "GONOSUMDB=*", "GOINSECURE=*"), args("version"), "fake go1.27.0")
}

func TestOldGoLine(t *testing.T) {
	e := newTestEnv(t)
	e.write("m/go.mod", "module m\n\ngo 1.16\n")
	e.fail("m", nil, args("version"), "go.mod selects go1.16, which predates toolchain switching")
	e.fail("m", env("GOTOOLCHAIN=path"), args("version"), "go.mod selects go1.16, which predates toolchain switching")
	// When GOTOOLCHAIN names a version, upstream selection applies.
	e.ok("m", env("GOTOOLCHAIN=go1.15+auto"), args("version"), "fake go1.16")
	e.ok("m", env("GOTOOLCHAIN=go1.27.3+auto"), args("version"), "fake go1.27.3")
}

func TestDefaultToolchain(t *testing.T) {
	e := newTestEnv(t)
	list := "/golang.org/toolchain/@v/list"
	cache := filepath.Join(e.root, "gopath", "pkg", "mod", "cache", "goup", "default-toolchain")

	// With no go.mod, the latest patch of the previous release runs.
	e.ok("", env("GODEBUG=toolchaintrace=1"), args("version"),
		"go: default toolchain set to go1.26.10 from the list of releases", "fake go1.26.10")
	if n := e.proxy.count(list); n != 1 {
		t.Errorf("version list fetched %d times, want 1", n)
	}
	data, err := os.ReadFile(cache)
	if err != nil || !strings.HasPrefix(string(data), "go1.26.10 ") {
		t.Fatalf("cache: %q, %v", data, err)
	}

	// A go.mod without a go line doesn't select a toolchain either.
	e.write("m/go.mod", "module m\n")
	e.ok("m", nil, args("version"), "fake go1.26.10")

	// The choice is reused for a day.
	e.proxy.setListFail(true)
	e.ok("", env("GODEBUG=toolchaintrace=1"), args("version"), "go: default toolchain set to go1.26.10 (chosen ", "fake go1.26.10")
	if n := e.proxy.count(list); n != 1 {
		t.Errorf("version list fetched %d times, want 1", n)
	}

	// An expired choice is refreshed.
	e.proxy.setListFail(false)
	old := "go1.26.9 " + time.Now().Add(-25*time.Hour).UTC().Format(time.RFC3339) + "\n"
	if err := os.WriteFile(cache, []byte(old), 0o666); err != nil {
		t.Fatal(err)
	}
	e.ok("", nil, args("version"), "fake go1.26.10")
	if n := e.proxy.count(list); n != 2 {
		t.Errorf("version list fetched %d times, want 2", n)
	}

	// An expired choice is reused if refreshing fails.
	e.proxy.setListFail(true)
	if err := os.WriteFile(cache, []byte(old), 0o666); err != nil {
		t.Fatal(err)
	}
	e.ok("", env("GODEBUG=toolchaintrace=1"), args("version"), "refreshing failed", "fake go1.26.9")

	// Without any choice, failing to fetch the list is an error.
	if err := os.Remove(cache); err != nil {
		t.Fatal(err)
	}
	e.fail("", nil, args("version"), "choosing a default toolchain", "go env -w GOTOOLCHAIN=")

	// In path mode, the default comes from the toolchains in PATH.
	e.pathToolchain("go1.21.0")
	e.pathToolchain("go1.27.3")
	e.ok("", env("GOTOOLCHAIN=path"), args("version"), "path go1.21.0")
}

func TestErrors(t *testing.T) {
	e := newTestEnv(t)
	e.write("m/go.mod", "module m\n\ngo 1.21\n")
	e.write("new/go.mod", "module m\n\ngo 1.99.0\n")
	e.fail("m", env("GOTOOLCHAIN=local"), args("version"), "GOTOOLCHAIN=local is not supported")
	e.fail("m", env("GO111MODULE=off"), args("version"), "GOPATH mode is not supported (GO111MODULE=off)")
	e.fail("", env("GO111MODULE=auto"), args("version"), "GOPATH mode is not supported (GO111MODULE=auto)")
	e.fail("m", env("GOTOOLCHAIN=bash"), args("version"), `invalid GOTOOLCHAIN "bash"`)
	e.fail("new", nil, args("version"), "download go1.99.0 for "+runtime.GOOS+"/"+runtime.GOARCH+": toolchain not available")
	// In module mode, GO111MODULE=auto works.
	e.ok("m", env("GO111MODULE=auto"), args("version"), "fake go1.21.0")
}

func TestEnvGOTOOLCHAIN(t *testing.T) {
	e := newTestEnv(t)
	goenv := filepath.Join(e.root, "goenv")
	e.ok("", nil, args("env GOTOOLCHAIN"), "auto")

	e.ok("", nil, args("env -w GOTOOLCHAIN=go1.27.0+auto"))
	if data, _ := os.ReadFile(goenv); string(data) != "GOTOOLCHAIN=go1.27.0+auto\n" {
		t.Errorf("go env file: %q", data)
	}
	e.ok("", nil, args("env GOTOOLCHAIN"), "go1.27.0+auto")
	e.ok("", nil, args("version"), "fake go1.27.0")

	// A bad setting can always be fixed.
	e.ok("", nil, args("env -w GOTOOLCHAIN=go1.99.0"))
	e.fail("", nil, args("version"), "toolchain not available")
	out := e.ok("", env("GOTOOLCHAIN=go1.99.0"), args("env -w GOTOOLCHAIN=go1.26.9"),
		"does not override conflicting OS environment variable")
	if strings.Contains(out, "fake") {
		t.Errorf("go env -w ran a toolchain:\n%s", out)
	}
	e.ok("", nil, args("version"), "fake go1.26.9")

	e.ok("", nil, args("env -u GOTOOLCHAIN"))
	if data, _ := os.ReadFile(goenv); strings.Contains(string(data), "GOTOOLCHAIN") {
		t.Errorf("go env file: %q", data)
	}
	e.ok("", nil, args("env GOTOOLCHAIN"), "auto")

	// Other go env command lines run a toolchain.
	e.ok("", nil, args("env GOMOD"), "fake go1.26.10 args=[env GOMOD]")
	e.ok("", nil, args("env -w GOTOOLCHAIN=auto GOPROXY=off"), "fake go1.26.10")
}

var network = flag.Bool("network", false, "run tests that download real toolchains")

func TestNetwork(t *testing.T) {
	if !*network {
		t.Skip("use -network to download a real toolchain from proxy.golang.org")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				os.Chmod(path, 0o777)
			}
			return nil
		})
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const tv = "go1.27.3"
	for range 2 {
		cmd := exec.Command(exe, "version")
		cmd.Dir = root
		cmd.Env = []string{
			"GOUP_TEST_MAIN=1",
			"HOME=" + root,
			"PATH=" + filepath.Join(root, "bin"),
			"GOPATH=" + filepath.Join(root, "gopath"),
			"GOENV=off",
			"GOTOOLCHAIN=" + tv,
		}
		if runtime.GOOS == "windows" {
			// Networking needs SYSTEMROOT, and temporary files need TEMP.
			for _, k := range []string{"SYSTEMROOT", "TEMP", "TMP"} {
				if v, ok := os.LookupEnv(k); ok {
					cmd.Env = append(cmd.Env, k+"="+v)
				}
			}
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go version: %v\n%s", err, out)
		}
		if want := "go version " + tv + " " + runtime.GOOS + "/" + runtime.GOARCH; !strings.Contains(string(out), want) {
			t.Errorf("go version: output doesn't contain %q:\n%s", want, out)
		}
	}
}
