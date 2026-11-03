//go:build unix

// Package unix is a stand-in for the parts of the standard library's
// internal/syscall/unix used by goup.
package unix

import "syscall"

const X_OK = 0x1

// Eaccess always returns ENOSYS, on which callers fall back to checking the
// permission bits, like os/exec does.
func Eaccess(path string, mode uint32) error {
	return syscall.ENOSYS
}
