//go:build windows

package runner

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	lockfileExclusiveLock   = 0x2
	lockfileFailImmediately = 0x1
	errorLockViolation      = syscall.Errno(33)
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = kernel32.NewProc("LockFileEx")
	procUnlockFile = kernel32.NewProc("UnlockFileEx")
)

func tryLock(f *os.File) (bool, error) {
	err := lockFileEx(f, lockfileExclusiveLock|lockfileFailImmediately)
	if errors.Is(err, errorLockViolation) {
		return false, nil
	}
	return err == nil, err
}

func lock(f *os.File) error {
	return lockFileEx(f, lockfileExclusiveLock)
}

func unlock(f *os.File) {
	var ov syscall.Overlapped
	_, _, _ = procUnlockFile.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&ov)))
}

// lockFileEx locks the first byte of f, which is all the mutual exclusion
// needs; the file's content is not covered by the region and stays writable
// by the holder.
func lockFileEx(f *os.File, flags uint32) error {
	var ov syscall.Overlapped
	r, _, err := procLockFileEx.Call(f.Fd(), uintptr(flags), 0, 1, 0, uintptr(unsafe.Pointer(&ov)))
	if r == 0 {
		return err
	}
	return nil
}
