//go:build unix

// Package unix is a stand-in for the parts of the standard library's
// internal/syscall/unix used by goup.
package unix

import (
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

const X_OK = 0x1

// Eaccess checks whether the effective user and group can access path with
// mode, following symlinks.
func Eaccess(path string, mode uint32) error {
	if runtime.GOOS == "android" {
		// syscall.Faccessat for Android implements AT_EACCESS check in
		// userspace. Since Android doesn't have setuid programs and
		// never runs code with euid!=uid, AT_EACCESS check is not
		// really required. Return ENOSYS so the callers can fall back
		// to permission bits check.
		return syscall.ENOSYS
	}
	return unix.Faccessat(unix.AT_FDCWD, path, mode, atEACCESS)
}
