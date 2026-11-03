// Command go selects, downloads, and runs the Go toolchain directed by
// GOTOOLCHAIN and the go.mod or go.work file, like the go command of a Go
// installation does, but without providing a toolchain of its own.
//
// It is built from the toolchain selection code of the upstream go command.
// See https://go.dev/doc/toolchain for how toolchains are selected, and the
// goup README for how goup differs.
package main

import (
	"log"
	"os"

	"geomys.org/goup/internal/go/base"
	"geomys.org/goup/internal/go/envcmd"
	"geomys.org/goup/internal/go/toolchain"
)

func main() {
	log.SetFlags(0)
	handleChdirFlag()
	if envcmd.RunGOTOOLCHAIN(os.Args[1:]) {
		base.Exit()
	}
	toolchain.Select()
	// Select returns only if GOTOOLCHAIN is empty, which go.env prevents.
	base.Fatalf("go: no toolchain selected")
}
