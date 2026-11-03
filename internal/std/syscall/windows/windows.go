//go:build windows

package windows

import (
	"syscall"
	"unsafe"
)

const (
	ERROR_SHARING_VIOLATION syscall.Errno = 32

	LOCKFILE_EXCLUSIVE_LOCK = 0x00000002
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

func LockFileEx(file syscall.Handle, flags uint32, reserved uint32, bytesLow uint32, bytesHigh uint32, overlapped *syscall.Overlapped) error {
	r1, _, e1 := syscall.SyscallN(procLockFileEx.Addr(), uintptr(file), uintptr(flags), uintptr(reserved), uintptr(bytesLow), uintptr(bytesHigh), uintptr(unsafe.Pointer(overlapped)))
	if r1 == 0 {
		return e1
	}
	return nil
}

func UnlockFileEx(file syscall.Handle, reserved uint32, bytesLow uint32, bytesHigh uint32, overlapped *syscall.Overlapped) error {
	r1, _, e1 := syscall.SyscallN(procUnlockFileEx.Addr(), uintptr(file), uintptr(reserved), uintptr(bytesLow), uintptr(bytesHigh), uintptr(unsafe.Pointer(overlapped)))
	if r1 == 0 {
		return e1
	}
	return nil
}
