package modload

import "geomys.org/goup/internal/go/modfetch"

// A Loader holds the subset of the upstream module loader state used by the
// functions kept in init.go.
type Loader struct {
	initialized bool

	// RootMode determines whether a module root is needed.
	RootMode Root

	modRoots []string
	fetcher  *modfetch.Fetcher
}
