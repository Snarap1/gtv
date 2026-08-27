package stats

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokensAndSaved(t *testing.T) {
	if Tokens(400) != 100 {
		t.Fatalf("Tokens(400) = %d", Tokens(400))
	}
	if Saved(4000, 400) != 900 {
		t.Fatalf("Saved = %d, want 900", Saved(4000, 400))
	}
	if Saved(100, 500) != 0 {
		t.Fatalf("Saved must floor at 0")
	}
	if Percent(4000, 400) != 90 {
		t.Fatalf("Percent = %d, want 90", Percent(4000, 400))
	}
	if Percent(0, 0) != 0 {
		t.Fatalf("Percent of empty baseline must be 0")
	}
}

func TestRecordPersistsAndAccumulates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, ".cache"))

	sessionMu.Lock()
	session = Snapshot{}
	sessionMu.Unlock()

	root := filepath.Join(dir, "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Record(root, 4000, 400); err != nil {
		t.Fatal(err)
	}
	if err := Record(root, 2000, 200); err != nil {
		t.Fatal(err)
	}

	f, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if f.Runs != 2 || f.BaselineChars != 6000 || f.ActualChars != 600 {
		t.Fatalf("all-time = %+v", f.Snapshot)
	}
	if len(f.Projects) != 1 {
		t.Fatalf("projects = %d", len(f.Projects))
	}
	for _, p := range f.Projects {
		if p.Root != filepath.Clean(root) {
			t.Fatalf("project root = %q", p.Root)
		}
		if p.Runs != 2 || Saved(p.BaselineChars, p.ActualChars) != 1350 {
			t.Fatalf("project = %+v", p)
		}
	}

	sess := Session()
	if sess.Runs != 2 || Saved(sess.BaselineChars, sess.ActualChars) != 1350 {
		t.Fatalf("session = %+v", sess)
	}

	out := Format(f, sess)
	for _, want := range []string{"all time", "this session", "avg saved/run", root} {
		if !strings.Contains(out, want) {
			t.Fatalf("Format missing %q:\n%s", want, out)
		}
	}
}
