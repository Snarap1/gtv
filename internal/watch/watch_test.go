package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUntilRerunsOnChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(file, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	runs := make(chan struct{}, 8)
	go Until(dir, func() { runs <- struct{}{} })

	select {
	case <-runs:
	case <-time.After(2 * time.Second):
		t.Fatal("Until did not run immediately")
	}

	// mtimes commonly have 1s resolution; make sure the write is observably later.
	time.Sleep(1100 * time.Millisecond)
	if err := os.WriteFile(file, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case <-runs:
	case <-time.After(3 * time.Second):
		t.Fatal("Until did not rerun after a file change")
	}
}

func TestSnapshotIgnoresSkippedDirs(t *testing.T) {
	dir := t.TempDir()
	buildDir := filepath.Join(dir, "build")
	// build/ itself must exist before the baseline snapshot: creating it here
	// would touch dir's own mtime, which snapshot does observe.
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	before := snapshot(dir)

	time.Sleep(1100 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(buildDir, "out.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := snapshot(dir); !sameSnapshot(got, before) {
		t.Fatal("snapshot changed after a write under build/, want it ignored")
	}
}
