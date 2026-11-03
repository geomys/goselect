//go:build windows

package windows

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	ERROR_SHARING_VIOLATION = windows.ERROR_SHARING_VIOLATION

	LOCKFILE_EXCLUSIVE_LOCK = windows.LOCKFILE_EXCLUSIVE_LOCK
)

// LockFileEx and UnlockFileEx take syscall types, like their counterparts in
// the standard library. windows.Overlapped has the same layout as
// syscall.Overlapped.

func LockFileEx(file syscall.Handle, flags uint32, reserved uint32, bytesLow uint32, bytesHigh uint32, overlapped *syscall.Overlapped) error {
	return windows.LockFileEx(windows.Handle(file), flags, reserved, bytesLow, bytesHigh, (*windows.Overlapped)(unsafe.Pointer(overlapped)))
}

func UnlockFileEx(file syscall.Handle, reserved uint32, bytesLow uint32, bytesHigh uint32, overlapped *syscall.Overlapped) error {
	return windows.UnlockFileEx(windows.Handle(file), reserved, bytesLow, bytesHigh, (*windows.Overlapped)(unsafe.Pointer(overlapped)))
}
