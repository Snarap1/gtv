package runner

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestAcquireNoWaitReportsBusy(t *testing.T) {
	root := t.TempDir()

	first, err := Acquire(root, false, nil)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Release()

	if _, err := Acquire(root, false, nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Acquire err = %v, want ErrBusy", err)
	}

	first.Release()
	second, err := Acquire(root, false, nil)
	if err != nil {
		t.Fatalf("Acquire after Release: %v", err)
	}
	second.Release()
}

func TestAcquireWaitsForHolder(t *testing.T) {
	root := t.TempDir()

	first, err := Acquire(root, false, nil)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	waited := make(chan int, 1)
	got := make(chan error, 1)
	go func() {
		l, err := Acquire(root, true, func(pid int) { waited <- pid })
		if l != nil {
			l.Release()
		}
		got <- err
	}()

	select {
	case pid := <-waited:
		if pid != os.Getpid() {
			t.Errorf("onWait pid = %d, want %d", pid, os.Getpid())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("onWait not called while lock held")
	}
	select {
	case err := <-got:
		t.Fatalf("second Acquire returned %v before Release", err)
	case <-time.After(100 * time.Millisecond):
	}

	first.Release()
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("second Acquire: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second Acquire did not proceed after Release")
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	l, err := Acquire(t.TempDir(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	l.Release()
}
