package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrBusy is returned by Acquire when another gtv holds the project lock and
// the caller asked not to wait.
var ErrBusy = errors.New("another gtv run is in progress")

// Lock serialises Gradle invocations per project root. Concurrent builds in
// one working tree share every task's outputs (build/classes, test-results,
// execution history) and Gradle takes no cross-process lock on them, so two
// gtv runs in the same repo race on the same files. The lock is an OS-level
// advisory file lock: a dead holder releases it automatically.
type Lock struct {
	f *os.File
}

// LockPath is the lock file for the given Gradle root.
func LockPath(root string) string {
	return filepath.Join(root, ".gradle", "gtv.lock")
}

// Acquire takes the project lock for root. When it is held elsewhere and
// wait is false, ErrBusy is returned; otherwise onWait (if set) is called
// once with the holder's recorded pid and Acquire blocks until the lock is
// free.
func Acquire(root string, wait bool, onWait func(holderPID int)) (*Lock, error) {
	path := LockPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}

	locked, err := tryLock(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("locking %s: %w", path, err)
	}
	if !locked {
		if !wait {
			f.Close()
			return nil, ErrBusy
		}
		if onWait != nil {
			onWait(readPID(path))
		}
		if err := lock(f); err != nil {
			f.Close()
			return nil, fmt.Errorf("locking %s: %w", path, err)
		}
	}

	l := &Lock{f: f}
	if err := l.record(); err != nil {
		l.Release()
		return nil, err
	}
	return l, nil
}

// Release drops the lock. Safe to call more than once.
func (l *Lock) Release() {
	if l.f == nil {
		return
	}
	unlock(l.f)
	l.f.Close()
	l.f = nil
}

// record writes the holder's pid into the lock file so a waiting gtv can
// name it. Purely informational: liveness is decided by the OS lock, not
// by this content. The record is fixed-width so it always overwrites the
// previous holder's without truncating the locked file.
func (l *Lock) record() error {
	_, err := l.f.WriteAt([]byte(fmt.Sprintf("%-20d\n", os.Getpid())), 0)
	return err
}

func readPID(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0
	}
	return pid
}
