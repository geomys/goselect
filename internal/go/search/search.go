// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package search

import (
	"path/filepath"

	"geomys.org/goup/internal/go/str"
)

// InDir checks whether path is in the file tree rooted at dir.
// If so, InDir returns an equivalent path relative to dir.
// If not, InDir returns an empty string.
// InDir makes some effort to succeed even in the presence of symbolic links.
func InDir(path, dir string) string {
	// inDirLex reports whether path is lexically in dir,
	// without considering symbolic or hard links.
	inDirLex := func(path, dir string) (string, bool) {
		if dir == "" {
			return path, true
		}
		rel := str.TrimFilePathPrefix(path, dir)
		if rel == path {
			return "", false
		}
		if rel == "" {
			return ".", true
		}
		return rel, true
	}

	if rel, ok := inDirLex(path, dir); ok {
		return rel
	}
	xpath, err := filepath.EvalSymlinks(path)
	if err != nil || xpath == path {
		xpath = ""
	} else {
		if rel, ok := inDirLex(xpath, dir); ok {
			return rel
		}
	}

	xdir, err := filepath.EvalSymlinks(dir)
	if err == nil && xdir != dir {
		if rel, ok := inDirLex(path, xdir); ok {
			return rel
		}
		if xpath != "" {
			if rel, ok := inDirLex(xpath, xdir); ok {
				return rel
			}
		}
	}
	return ""
}
