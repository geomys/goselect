// Command rewrite transforms upstream Go source files into their goup
// form. pull.sh runs it, through git filter-repo, on every version of every
// Go file in the filtered upstream history, so its output must depend only on
// the file name and contents.
//
// It rewrites import paths of cmd/go internal packages, cmd/internal packages,
// and standard library internal packages to their goup locations. For a
// few files, of which goup needs only a handful of declarations, it also
// drops every other top-level declaration and the imports that become unused.
// Rewritten files are formatted with gofmt.
//
// Usage:
//
//	rewrite -name FILENAME <in >out
//	rewrite -server
//
// In server mode, rewrite reads a sequence of requests from standard input,
// each made of a header line "NAMELEN DATALEN\n" followed by the file name and
// the file contents, and answers each with a header line "OUTLEN\n" followed
// by the rewritten contents.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// importRewrites maps upstream import path prefixes to goup ones.
// The first matching prefix applies.
var importRewrites = []struct{ old, new string }{
	{"cmd/go/internal/", "geomys.org/goup/internal/go/"},
	{"cmd/internal/", "geomys.org/goup/internal/cmd/"},
	{"internal/", "geomys.org/goup/internal/std/"},
}

// extractions lists, by file name after the pull.sh path renames, the files
// of which only the named top-level declarations are kept. A method is named
// as "Type.Method". A const, var, or type block is kept whole if it declares
// any of the names.
var extractions = map[string][]string{
	"internal/go/modload/init.go": {
		"Root", "AutoRoot",
		"NewLoader", "Loader.Fetcher",
		"Loader.FindGoWork", "Loader.WillBeEnabled",
		"FindGoMod", "findModuleRoot", "findWorkspaceFile",
	},
	"internal/go/search/search.go": {
		"InDir",
	},
	"internal/go/envcmd/env.go": {
		"checkEnvWrite", "readEnvFileLines", "updateEnvFile",
		"lineToKey", "sortKeyValues",
	},
	"cmd/go/chdir.go": {
		"handleChdirFlag", "lookupCmd",
	},
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("rewrite: ")
	name := flag.String("name", "", "file name, after path renames")
	server := flag.Bool("server", false, "process a stream of requests")
	flag.Parse()

	if *server {
		if err := serve(os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *name == "" {
		flag.Usage()
		os.Exit(2)
	}
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.Write(Rewrite(*name, src))
}

func serve(r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)
	bw := bufio.NewWriter(w)
	for {
		line, err := br.ReadString('\n')
		if err == io.EOF && line == "" {
			return nil
		}
		if err != nil {
			return err
		}
		var nameLen, dataLen int
		if _, err := fmt.Sscanf(line, "%d %d\n", &nameLen, &dataLen); err != nil {
			return fmt.Errorf("malformed request header %q: %v", line, err)
		}
		buf := make([]byte, nameLen+dataLen)
		if _, err := io.ReadFull(br, buf); err != nil {
			return err
		}
		out := Rewrite(string(buf[:nameLen]), buf[nameLen:])
		fmt.Fprintf(bw, "%d\n", len(out))
		bw.Write(out)
		if err := bw.Flush(); err != nil {
			return err
		}
	}
}

// Rewrite returns the goup form of the file name with contents src.
// Files that are not Go source files are returned unchanged.
func Rewrite(name string, src []byte) []byte {
	if !strings.HasSuffix(name, ".go") {
		return src
	}
	if names, ok := extractions[name]; ok {
		out, err := extract(src, names)
		if err == nil {
			return out
		}
		log.Printf("%s: %v; rewriting imports only", name, err)
	}
	out, err := rewriteImports(src)
	if err != nil {
		log.Printf("%s: %v; rewriting import paths as text", name, err)
		return rewriteImportsText(src)
	}
	return out
}

func rewritePath(p string) (string, bool) {
	for _, r := range importRewrites {
		if rest, ok := strings.CutPrefix(p, r.old); ok {
			return r.new + rest, true
		}
	}
	return p, false
}

// rewriteImports rewrites the import paths of src, and formats it if any
// changed, so that import blocks stay sorted.
func rewriteImports(src []byte) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ImportsOnly|parser.ParseComments)
	if err != nil {
		return nil, err
	}
	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		if np, ok := rewritePath(p); ok {
			edits = append(edits, edit{
				start: fset.Position(imp.Path.Pos()).Offset,
				end:   fset.Position(imp.Path.End()).Offset,
				text:  strconv.Quote(np),
			})
		}
	}
	if len(edits) == 0 {
		return src, nil
	}
	var buf bytes.Buffer
	last := 0
	for _, e := range edits {
		buf.Write(src[last:e.start])
		buf.WriteString(e.text)
		last = e.end
	}
	buf.Write(src[last:])
	out, err := format.Source(regroupImports(buf.Bytes()))
	if err != nil {
		// Keep the rewritten paths even if the file can't be formatted.
		return buf.Bytes(), nil
	}
	return out, nil
}

// regroupImports moves the non-standard-library imports of each blank-line
// separated group of a parenthesized import declaration that also has
// standard library imports into a new group right after it, like goimports
// does. Without this, imports rewritten from standard library internal
// packages would stay among the standard library ones, and every goimports
// run on a modified file would regroup them.
func regroupImports(src []byte) []byte {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ImportsOnly|parser.ParseComments)
	if err != nil {
		return src
	}
	var out bytes.Buffer
	last := 0
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT || !gd.Lparen.IsValid() {
			continue
		}
		// The block contents, from the line after "import (" up to the
		// line of ")".
		start := fset.Position(gd.Lparen).Offset + 1
		end := fset.Position(gd.Rparen).Offset
		if i := bytes.IndexByte(src[start:end], '\n'); i >= 0 {
			start += i + 1
		} else {
			continue
		}
		end = bytes.LastIndexByte(src[:end], '\n') + 1
		if end <= start {
			continue
		}
		out.Write(src[last:start])
		out.Write(regroupBlock(src[start:end]))
		last = end
	}
	out.Write(src[last:])
	return out.Bytes()
}

// regroupBlock regroups the lines of the contents of an import block.
func regroupBlock(block []byte) []byte {
	lines := strings.SplitAfter(string(block), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var out strings.Builder
	var std, other []string
	var pending []string // comment lines, attached to the next import
	flush := func() {
		for _, l := range std {
			out.WriteString(l)
		}
		if len(std) > 0 && len(other) > 0 {
			out.WriteString("\n")
		}
		for _, l := range other {
			out.WriteString(l)
		}
		for _, l := range pending {
			out.WriteString(l)
		}
		std, other, pending = nil, nil, nil
	}
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		switch {
		case trimmed == "":
			flush()
			out.WriteString(l)
		case strings.HasPrefix(trimmed, "//"):
			pending = append(pending, l)
		default:
			item := strings.Join(pending, "") + l
			pending = nil
			if isStdImportLine(trimmed) {
				std = append(std, item)
			} else {
				other = append(other, item)
			}
		}
	}
	flush()
	return []byte(out.String())
}

var importLineRE = regexp.MustCompile(`"([^"]*)"`)

// isStdImportLine reports whether an import spec line imports a standard
// library package, judging by the absence of a dot in its first element.
// Lines it can't understand count as standard library ones, which leaves
// them in place.
func isStdImportLine(line string) bool {
	m := importLineRE.FindStringSubmatch(line)
	if m == nil {
		return true
	}
	first, _, _ := strings.Cut(m[1], "/")
	return !strings.Contains(first, ".")
}

var importPathRE = regexp.MustCompile(`"(cmd/go/internal/|cmd/internal/|internal/)`)

// rewriteImportsText is the fallback for files that don't parse.
func rewriteImportsText(src []byte) []byte {
	return importPathRE.ReplaceAllFunc(src, func(m []byte) []byte {
		p, _ := rewritePath(string(m[1:]))
		return []byte(`"` + p)
	})
}

// extract keeps only the named top-level declarations of src, along with the
// file header, the package clause, and the imports they use.
func extract(src []byte, names []string) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	// lineEnd returns the offset just past the end of the line containing i.
	lineEnd := func(i int) int {
		if j := bytes.IndexByte(src[i:], '\n'); j >= 0 {
			return i + j + 1
		}
		return len(src)
	}

	var kept []ast.Decl
	for _, d := range f.Decls {
		if slices.ContainsFunc(declNames(d), func(n string) bool {
			return slices.Contains(names, n)
		}) {
			kept = append(kept, d)
		}
	}

	used := make(map[string]bool)
	for _, d := range kept {
		ast.Inspect(d, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok {
					used[id.Name] = true
				}
			}
			return true
		})
	}

	var buf bytes.Buffer
	// Keep the header and package clause, minus any go:generate directives,
	// which would run upstream commands.
	for _, line := range strings.SplitAfter(string(src[:lineEnd(off(f.Name.End()))]), "\n") {
		if !strings.HasPrefix(line, "//go:generate ") {
			buf.WriteString(line)
		}
	}
	var std, other []string
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		name := importName(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "_" && name != "." && !used[name] {
			continue
		}
		np, _ := rewritePath(p)
		spec := strconv.Quote(np)
		if imp.Name != nil {
			spec = imp.Name.Name + " " + spec
		}
		if first, _, _ := strings.Cut(np, "/"); strings.Contains(first, ".") {
			other = append(other, spec)
		} else {
			std = append(std, spec)
		}
	}
	if len(std)+len(other) > 0 {
		buf.WriteString("\nimport (\n")
		for _, spec := range std {
			fmt.Fprintf(&buf, "\t%s\n", spec)
		}
		if len(std) > 0 && len(other) > 0 {
			buf.WriteString("\n")
		}
		for _, spec := range other {
			fmt.Fprintf(&buf, "\t%s\n", spec)
		}
		buf.WriteString(")\n")
	}
	for _, d := range kept {
		start := off(d.Pos())
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Doc != nil {
				start = off(d.Doc.Pos())
			}
		case *ast.GenDecl:
			if d.Doc != nil {
				start = off(d.Doc.Pos())
			}
		}
		buf.WriteString("\n")
		buf.Write(src[start:lineEnd(off(d.End()))])
	}
	return format.Source(buf.Bytes())
}

// declNames returns the names declared by a top-level declaration, with
// methods named as "Type.Method".
func declNames(d ast.Decl) []string {
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil || len(d.Recv.List) == 0 {
			return []string{d.Name.Name}
		}
		t := d.Recv.List[0].Type
		if st, ok := t.(*ast.StarExpr); ok {
			t = st.X
		}
		switch tt := t.(type) {
		case *ast.IndexExpr:
			t = tt.X
		case *ast.IndexListExpr:
			t = tt.X
		}
		if id, ok := t.(*ast.Ident); ok {
			return []string{id.Name + "." + d.Name.Name}
		}
		return nil
	case *ast.GenDecl:
		var names []string
		for _, s := range d.Specs {
			switch s := s.(type) {
			case *ast.TypeSpec:
				names = append(names, s.Name.Name)
			case *ast.ValueSpec:
				for _, n := range s.Names {
					names = append(names, n.Name)
				}
			}
		}
		return names
	}
	return nil
}

// importName returns the default package name for an import path.
func importName(p string) string {
	base := path.Base(p)
	if len(base) >= 2 && base[0] == 'v' && strings.Trim(base[1:], "0123456789") == "" {
		if dir := path.Dir(p); dir != "." {
			base = path.Base(dir)
		}
	}
	return base
}
