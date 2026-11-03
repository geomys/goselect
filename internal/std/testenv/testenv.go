// Package testenv is a stand-in for the parts of the standard library's
// internal/testenv used by the upstream tests kept in goup.
package testenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// Command is like exec.Command, but fails the test if exec is unsupported.
func Command(t testing.TB, name string, args ...string) *exec.Cmd {
	t.Helper()
	MustHaveExec(t)
	return exec.Command(name, args...)
}

// Executable returns the path to the test binary.
func Executable(t testing.TB) string {
	t.Helper()
	MustHaveExec(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return exe
}

// MustHaveExecPath skips the test if path can't be found in $PATH.
func MustHaveExecPath(t testing.TB, path string) {
	t.Helper()
	MustHaveExec(t)
	if _, err := exec.LookPath(path); err != nil {
		t.Skipf("skipping test: %s: %v", path, err)
	}
}

// MustHaveSymlink skips the test if symlinks can't be created.
func MustHaveSymlink(t testing.TB) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
		t.Skipf("skipping test: cannot make symlinks on %s/%s: %v", runtime.GOOS, runtime.GOARCH, err)
	}
}

// MustHaveExec skips the test if subprocesses can't be started.
func MustHaveExec(t testing.TB) {
	t.Helper()
	switch runtime.GOOS {
	case "js", "wasip1", "ios":
		t.Skipf("skipping test: cannot exec subprocess on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}
