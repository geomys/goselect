package envcmd

import (
	"fmt"
	"os"
	"strings"

	"geomys.org/goup/internal/go/base"
	"geomys.org/goup/internal/go/cfg"
)

// RunGOTOOLCHAIN runs the go env command lines that only concern GOTOOLCHAIN,
// without running a toolchain, so that a bad setting can always be fixed:
//
//	go env GOTOOLCHAIN
//	go env -w GOTOOLCHAIN=value
//	go env -u GOTOOLCHAIN
//
// It reports whether args, the command line arguments, are one of them.
// The first two are the command lines that upstream runs with the local
// toolchain, without toolchain selection.
func RunGOTOOLCHAIN(args []string) bool {
	switch {
	case len(args) == 2 && args[0] == "env" && args[1] == "GOTOOLCHAIN":
		fmt.Println(cfg.Getenv("GOTOOLCHAIN"))

	case len(args) == 3 && args[0] == "env" && args[1] == "-w" && strings.HasPrefix(args[2], "GOTOOLCHAIN="):
		// Like runEnvW.
		key, val, _ := strings.Cut(args[2], "=")
		if err := checkEnvWrite(key, val); err != nil {
			base.Fatal(err)
		}
		if osVal := os.Getenv(key); osVal != "" && osVal != val {
			fmt.Fprintf(os.Stderr, "warning: go env -w %s=... does not override conflicting OS environment variable\n", key)
		}
		updateEnvFile(map[string]string{key: val}, nil)

	case len(args) == 3 && args[0] == "env" && args[1] == "-u" && args[2] == "GOTOOLCHAIN":
		// Like runEnvU.
		if err := checkEnvWrite(args[2], ""); err != nil {
			base.Fatal(err)
		}
		updateEnvFile(nil, map[string]bool{args[2]: true})

	default:
		return false
	}
	return true
}
