//go:build unix && !aix

package unix

import "golang.org/x/sys/unix"

const atEACCESS = unix.AT_EACCESS
