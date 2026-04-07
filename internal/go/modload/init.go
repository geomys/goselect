// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package modload provides module and package loading functionality.
package modload

import (
	"os"
	"path/filepath"

	"geomys.org/goup/internal/go/cfg"
	"geomys.org/goup/internal/go/fsys"
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
