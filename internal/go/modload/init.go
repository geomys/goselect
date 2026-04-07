// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package modload provides module and package loading functionality.
package modload

import (
	"os"
	"path/filepath"

	"geomys.org/goup/internal/go/base"
	"geomys.org/goup/internal/go/cfg"
	"geomys.org/goup/internal/go/fsys"
	"geomys.org/goup/internal/go/modfetch"
	"geomys.org/goup/internal/go/search"
)

type Root int

const (
	// AutoRoot is the default for most commands. modload.Init will look for
	// a go.mod file in the current directory or any parent. If none is found,
	// modules may be disabled (GO111MODULE=auto) or commands may run in a
	// limited module mode.
	AutoRoot Root = iota

	// NoRoot is used for commands that run in module mode and ignore any go.mod
	// file the current directory or in parent directories.
	NoRoot

	// NeedRoot is used for commands that must run in module mode and don't
	// make sense without a main module.
	NeedRoot
)

// FindGoWork returns the name of the go.work file for this command,
// or the empty string if there isn't one.
// Most code should use Init and Enabled rather than use this directly.
// It is exported mainly for Go toolchain switching, which must process
// the go.work very early at startup.
func (ld *Loader) FindGoWork(wd string) string {
	if ld.RootMode == NoRoot {
		return ""
	}

	switch gowork := cfg.Getenv("GOWORK"); gowork {
	case "off":
		return ""
	case "", "auto":
		return findWorkspaceFile(wd)
	default:
		if !filepath.IsAbs(gowork) {
			base.Fatalf("go: invalid GOWORK: not an absolute path")
		}
		return gowork
	}
}

func NewLoader() *Loader {
	s := new(Loader)
	s.fetcher = modfetch.NewFetcher()
	return s
}

func (ld *Loader) Fetcher() *modfetch.Fetcher {
	return ld.fetcher
}

// WillBeEnabled checks whether modules should be enabled but does not
// initialize modules by installing hooks. If Init has already been called,
// WillBeEnabled returns the same result as Enabled.
//
// This function is needed to break a cycle. The main package needs to know
// whether modules are enabled in order to install the module or GOPATH version
// of 'go get', but Init reads the -modfile flag in 'go get', so it shouldn't
// be called until the command is installed and flags are parsed. Instead of
// calling Init and Enabled, the main package can call this function.
func (ld *Loader) WillBeEnabled() bool {
	if ld.modRoots != nil || cfg.ModulesEnabled {
		// Already enabled.
		return true
	}
	if ld.initialized {
		// Initialized, not enabled.
		return false
	}

	// Keep in sync with Init. Init does extra validation and prints warnings or
	// exits, so it can't call this function directly.
	env := cfg.Getenv("GO111MODULE")
	switch env {
	case "on", "":
		return true
	case "auto":
		break
	default:
		return false
	}

	return FindGoMod(base.Cwd()) != "" || ld.FindGoWork(base.Cwd()) != ""
}

// FindGoMod returns the name of the go.mod file for this command,
// or the empty string if there isn't one.
// Most code should use Init and Enabled rather than use this directly.
// It is exported mainly for Go toolchain switching, which must process
// the go.mod very early at startup.
func FindGoMod(wd string) string {
	modRoot := findModuleRoot(wd)
	if modRoot == "" {
		// GO111MODULE is 'auto', and we can't find a module root.
		// Stay in GOPATH mode.
		return ""
	}
	if search.InDir(modRoot, os.TempDir()) == "." {
		// If you create /tmp/go.mod for experimenting,
		// then any tests that create work directories under /tmp
		// will find it and get modules when they're not expecting them.
		// It's a bit of a peculiar thing to disallow but quite mysterious
		// when it happens. See golang.org/issue/26708.
		return ""
	}
	return filepath.Join(modRoot, "go.mod")
}

func findModuleRoot(dir string) (roots string) {
	if dir == "" {
		panic("dir not set")
	}
	dir = filepath.Clean(dir)

	// Look for enclosing go.mod.
	for {
		if fi, err := fsys.Stat(filepath.Join(dir, "go.mod")); err == nil && !fi.IsDir() {
			return dir
		}
		d := filepath.Dir(dir)
		if d == dir {
			break
		}
		dir = d
	}
	return ""
}

func findWorkspaceFile(dir string) (root string) {
	if dir == "" {
		panic("dir not set")
	}
	dir = filepath.Clean(dir)

	// Look for enclosing go.mod.
	for {
		f := filepath.Join(dir, "go.work")
		if fi, err := fsys.Stat(f); err == nil && !fi.IsDir() {
			return f
		}
		d := filepath.Dir(dir)
		if d == dir {
			break
		}
		if d == cfg.GOROOT {
			// As a special case, don't cross GOROOT to find a go.work file.
			// The standard library and commands built in go always use the vendored
			// dependencies, so avoid using a most likely irrelevant go.work file.
			return ""
		}
		dir = d
	}
	return ""
}
