package toolchain

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"geomys.org/goup/internal/go/base"
	"geomys.org/goup/internal/go/cfg"
	"geomys.org/goup/internal/go/gover"
	"geomys.org/goup/internal/go/lockedfile"
	"geomys.org/goup/internal/go/modfetch"
)

// defaultMinVersion is the Go version that the default toolchain must
// support. Go 1.21 is the first release that implements toolchain switching.
const defaultMinVersion = "1.21"

// defaultCacheTTL is how long a default toolchain choice is reused.
const defaultCacheTTL = 24 * time.Hour

// defaultToolchain returns the toolchain to run when neither GOTOOLCHAIN nor
// a go.mod or go.work file selects one. It's the toolchain upstream switches
// to for a requirement of Go 1.21 (see [NewerToolchain]), usually the latest
// patch release of the previous major release.
//
// In auto mode, the choice is cached in the module cache for a day, so that
// it changes at most daily and doesn't need network access on every run.
// If the list of releases can't be fetched, an expired choice is reused.
func defaultToolchain(f *modfetch.Fetcher, trace io.Writer) string {
	if !HasAuto() {
		tv, err := NewerToolchain(context.Background(), f, defaultMinVersion)
		if err != nil {
			base.Fatalf("choosing a default toolchain: %v", err)
		}
		if toolchainTrace {
			fmt.Fprintf(trace, "go: default toolchain set to %s from the toolchains in PATH\n", tv)
		}
		return tv
	}

	file := defaultCacheFile()
	cached, at := readDefaultCache(file)
	age := time.Since(at)
	if cached != "" && age >= 0 && age < defaultCacheTTL {
		if toolchainTrace {
			fmt.Fprintf(trace, "go: default toolchain set to %s (chosen %v ago)\n", cached, age.Round(time.Second))
		}
		return cached
	}

	tv, err := NewerToolchain(context.Background(), f, defaultMinVersion)
	if err != nil {
		if cached != "" {
			if toolchainTrace {
				fmt.Fprintf(trace, "go: default toolchain set to %s (chosen %v ago; refreshing failed: %v)\n", cached, age.Round(time.Second), err)
			}
			return cached
		}
		base.Fatalf("choosing a default toolchain: %v\n"+
			"\tset one with 'go env -w GOTOOLCHAIN=go1.N.P+auto'", err)
	}
	writeDefaultCache(file, tv)
	if toolchainTrace {
		fmt.Fprintf(trace, "go: default toolchain set to %s from the list of releases\n", tv)
	}
	return tv
}

func defaultCacheFile() string {
	if cfg.GOMODCACHE == "" || !filepath.IsAbs(cfg.GOMODCACHE) {
		return ""
	}
	return filepath.Join(cfg.GOMODCACHE, "cache", "goup", "default-toolchain")
}

// readDefaultCache returns the cached default toolchain and when it was
// chosen, or "" if there is no valid cached choice.
func readDefaultCache(file string) (string, time.Time) {
	if file == "" {
		return "", time.Time{}
	}
	data, err := lockedfile.Read(file)
	if err != nil {
		return "", time.Time{}
	}
	tv, ts, ok := strings.Cut(strings.TrimSpace(string(data)), " ")
	if !ok || !strings.HasPrefix(tv, "go") || gover.FromToolchain(tv) == "" {
		return "", time.Time{}
	}
	at, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "", time.Time{}
	}
	return tv, at
}

// writeDefaultCache records tv as the default toolchain, on a best-effort basis.
func writeDefaultCache(file, tv string) {
	if file == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o777); err != nil {
		return
	}
	data := tv + " " + time.Now().UTC().Format(time.RFC3339) + "\n"
	lockedfile.Write(file, strings.NewReader(data), 0o666)
}
