package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestRewriteNonGo(t *testing.T) {
	src := []byte(`"cmd/go/internal/base"` + "\n")
	if out := Rewrite("internal/go/cfg/go.env", src); !bytes.Equal(out, src) {
		t.Errorf("non-Go file changed: %q", out)
	}
}

func TestRewriteUnchanged(t *testing.T) {
	// Not gofmt-formatted, to check that files without rewritten imports are
	// returned byte for byte.
	src := []byte("package p\n\nimport \"fmt\"\n\nvar   x = fmt.Sprint()\n")
	if out := Rewrite("internal/go/str/str.go", src); !bytes.Equal(out, src) {
		t.Errorf("file without rewritten imports changed:\n%s", out)
	}
}

func TestRewriteImports(t *testing.T) {
	src := `// Copyright header.

//go:build unix

package toolchain

import (
	"cmd/go/internal/base"
	"fmt"
	"internal/godebug"
	"os"

	"cmd/go/internal/gover"
	"cmd/internal/pathcache"
	web "cmd/go/internal/web"

	"golang.org/x/mod/module"
)

var _ = fmt.Sprint(base.Go, godebug.New, os.Args, gover.Local, pathcache.LookPath, web.Get, module.Version{})
`
	want := `// Copyright header.

//go:build unix

package toolchain

import (
	"fmt"
	"os"

	"geomys.org/goup/internal/go/base"
	"geomys.org/goup/internal/std/godebug"

	"geomys.org/goup/internal/cmd/pathcache"
	"geomys.org/goup/internal/go/gover"
	web "geomys.org/goup/internal/go/web"

	"golang.org/x/mod/module"
)

var _ = fmt.Sprint(base.Go, godebug.New, os.Args, gover.Local, pathcache.LookPath, web.Get, module.Version{})
`
	if out := string(Rewrite("internal/go/toolchain/exec.go", []byte(src))); out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestRewriteImportsText(t *testing.T) {
	// A file that doesn't parse falls back to rewriting quoted paths as text.
	src := "package p\n\nimport \"cmd/internal/par\"\n\nfunc (\n"
	want := "package p\n\nimport \"geomys.org/goup/internal/cmd/par\"\n\nfunc (\n"
	if out := string(Rewrite("internal/go/p/p.go", []byte(src))); out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestExtract(t *testing.T) {
	src := `// Copyright header.

// Package modload does things.
package modload

import (
	"errors"
	"os"
	"path/filepath"

	"cmd/go/internal/base"
	"cmd/go/internal/cfg"
	"cmd/go/internal/modfetch"
	_ "embed"
	"golang.org/x/mod/module"
)

// A Root is a root.
type Root int

const (
	// AutoRoot is automatic.
	AutoRoot Root = iota
	NoRoot
)

var unrelated = errors.New("unrelated")

// FindGoMod finds go.mod.
func FindGoMod(wd string) string {
	// An inner comment.
	return filepath.Join(wd, "go.mod") // A trailing comment.
}

// Loader loads.
type Loader struct {
	fetcher *modfetch.Fetcher
}

// Fetcher returns the fetcher.
func (ld *Loader) Fetcher() *modfetch.Fetcher {
	return ld.fetcher
}

func (ld *Loader) Other() module.Version {
	base.Fatalf("%v", cfg.GOROOT)
	return module.Version{}
}

func init() {
	os.Exit(0)
}
`
	want := `// Copyright header.

// Package modload does things.
package modload

import (
	_ "embed"
	"path/filepath"

	"geomys.org/goup/internal/go/modfetch"
)

// A Root is a root.
type Root int

const (
	// AutoRoot is automatic.
	AutoRoot Root = iota
	NoRoot
)

// FindGoMod finds go.mod.
func FindGoMod(wd string) string {
	// An inner comment.
	return filepath.Join(wd, "go.mod") // A trailing comment.
}

// Fetcher returns the fetcher.
func (ld *Loader) Fetcher() *modfetch.Fetcher {
	return ld.fetcher
}
`
	out, err := extract([]byte(src), []string{"Root", "AutoRoot", "FindGoMod", "Loader.Fetcher", "Missing"})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestExtractNothing(t *testing.T) {
	// Older upstream versions may lack all the requested declarations.
	src := "// Header.\n\npackage search\n\nimport \"fmt\"\n\nfunc F() { fmt.Println() }\n"
	want := "// Header.\n\npackage search\n"
	out, err := extract([]byte(src), []string{"InDir"})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

func TestServe(t *testing.T) {
	var in bytes.Buffer
	files := []struct{ name, src string }{
		{"internal/go/base/base.go", "package base\n\nimport \"cmd/internal/par\"\n\nvar _ par.Work\n"},
		{"internal/go/cfg/go.env", "GOTOOLCHAIN=auto\n"},
		{"empty.go", ""},
	}
	for _, f := range files {
		fmt.Fprintf(&in, "%d %d\n%s%s", len(f.name), len(f.src), f.name, f.src)
	}
	var out bytes.Buffer
	if err := serve(&in, &out); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		var n int
		if _, err := fmt.Fscanf(&out, "%d\n", &n); err != nil {
			t.Fatal(err)
		}
		got := string(out.Next(n))
		if want := string(Rewrite(f.name, []byte(f.src))); got != want {
			t.Errorf("%s: got %q, want %q", f.name, got, want)
		}
	}
	if out.Len() != 0 {
		t.Errorf("trailing output: %q", out.String())
	}
	if !strings.Contains(string(Rewrite(files[0].name, []byte(files[0].src))), "geomys.org/goup/internal/cmd/par") {
		t.Errorf("import not rewritten")
	}
}

func TestImportName(t *testing.T) {
	for p, want := range map[string]string{
		"fmt":                    "fmt",
		"golang.org/x/mod/zip":   "zip",
		"math/rand/v2":           "rand",
		"cmd/go/internal/gover":  "gover",
		"example.com/mod/v3/sub": "sub",
	} {
		if got := importName(p); got != want {
			t.Errorf("importName(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestExtractDropsGenerate(t *testing.T) {
	src := "// Header.\n\n//go:generate go test cmd/go -fixdocs\n\npackage main\n\nfunc lookupCmd() {}\n"
	want := "// Header.\n\npackage main\n\nfunc lookupCmd() {}\n"
	out, err := extract([]byte(src), []string{"lookupCmd"})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

func TestRegroupImports(t *testing.T) {
	src := `package p

import (
	"bytes"
	// A comment about godebug.
	"geomys.org/goup/internal/std/godebug" // Trailing.
	"io"

	"geomys.org/goup/internal/go/base"
	"strings"
)

import "geomys.org/goup/internal/std/cfg"
`
	want := `package p

import (
	"bytes"
	"io"

	// A comment about godebug.
	"geomys.org/goup/internal/std/godebug" // Trailing.

	"strings"

	"geomys.org/goup/internal/go/base"
)

import "geomys.org/goup/internal/std/cfg"
`
	if out := string(regroupImports([]byte(src))); out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}
