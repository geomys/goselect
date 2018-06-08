// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bitbucket

import (
	"fmt"
	"strings"

	"geomys.org/goup/internal/go/modfetch/codehost"
	"geomys.org/goup/internal/go/modfetch/gitrepo"
)

func Lookup(path string) (codehost.Repo, error) {
	f := strings.Split(path, "/")
	if len(f) < 3 || f[0] != "bitbucket.org" {
		return nil, fmt.Errorf("bitbucket repo must be bitbucket.org/org/project")
	}
	path = f[0] + "/" + f[1] + "/" + f[2]
	return gitrepo.Repo("https://"+path, path)
}
