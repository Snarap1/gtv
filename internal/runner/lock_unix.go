//go:build unix

package runner

import (
	"errors"
	"os"
	"syscall"
)

func tryLock(f *os.File) (bool, error) {
	err := flock(f, syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func lock(f *os.File) error {
	return flock(f, syscall.LOCK_EX)
}

func unlock(f *os.File) {
	_ = flock(f, syscall.LOCK_UN)
}

// flock retries on EINTR: a blocking flock(2) is interrupted by the Go
// runtime's preemption signals.
func flock(f *os.File, how int) error {
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}
