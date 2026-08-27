// Package stats tracks how many tokens gtv saves versus raw Gradle console
// output across runs. Baseline is the full Gradle stdout+stderr byte count
// from the same invocation (not a second launch, not an NDJSON estimate);
// actual is the agent-renderer output for that same tree.
package stats

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Snapshot is a cumulative counters set: either global, per-project, or
// process-local (session).
type Snapshot struct {
	BaselineChars int64 `json:"baseline_chars"`
	ActualChars   int64 `json:"actual_chars"`
	Runs          int64 `json:"runs"`
}

// Project holds one Gradle root's counters.
type Project struct {
	Root string `json:"root"`
	Snapshot
}

// File is the on-disk shape of ~/.cache/gtv/stats.json.
type File struct {
	Snapshot
	Projects map[string]Project `json:"projects"`
}

// session is process-local; useful across --watch reruns in one gtv process.
var (
	sessionMu sync.Mutex
	session   Snapshot
)

// Tokens estimates LLM tokens from a character count (chars/4).
func Tokens(chars int64) int64 {
	if chars < 0 {
		return 0
	}
	return chars / 4
}

// Saved returns baseline−actual tokens, floored at zero.
func Saved(baseline, actual int64) int64 {
	b, a := Tokens(baseline), Tokens(actual)
	if a >= b {
		return 0
	}
	return b - a
}

// Percent is the fraction of baseline tokens avoided, 0–100.
func Percent(baseline, actual int64) int {
	b := Tokens(baseline)
	if b == 0 {
		return 0
	}
	return int(Saved(baseline, actual) * 100 / b)
}

// Record adds one run's baseline (Gradle console bytes) and actual (agent
// report bytes) to disk and to the process session. A cache write failure is
// returned but the session counter still advances.
func Record(root string, baseline, actual int64) error {
	sessionMu.Lock()
	session.BaselineChars += baseline
	session.ActualChars += actual
	session.Runs++
	sessionMu.Unlock()

	path, err := filePath()
	if err != nil {
		return err
	}
	f, err := load(path)
	if err != nil {
		f = &File{Projects: map[string]Project{}}
	}
	if f.Projects == nil {
		f.Projects = map[string]Project{}
	}

	f.BaselineChars += baseline
	f.ActualChars += actual
	f.Runs++

	key := rootKey(root)
	p := f.Projects[key]
	p.Root = filepath.Clean(root)
	p.BaselineChars += baseline
	p.ActualChars += actual
	p.Runs++
	f.Projects[key] = p

	return save(path, f)
}

// Session returns a copy of the process-local counters.
func Session() Snapshot {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	return session
}

// Load reads the persisted stats file. Missing file yields an empty File.
func Load() (*File, error) {
	path, err := filePath()
	if err != nil {
		return nil, err
	}
	return load(path)
}

// Format renders a human-readable summary of all-time, session, and projects.
func Format(f *File, sess Snapshot) string {
	if f == nil {
		f = &File{}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "gtv token savings (≈ chars/4)\n")
	writeBlock(&b, "all time", f.Snapshot)
	if sess.Runs > 0 {
		writeBlock(&b, "this session", sess)
	}
	if f.Runs > 0 {
		avg := Percent(f.BaselineChars, f.ActualChars)
		fmt.Fprintf(&b, "  avg saved/run: %d%%\n", avg)
	}
	if len(f.Projects) > 0 {
		fmt.Fprintf(&b, "by project:\n")
		keys := make([]string, 0, len(f.Projects))
		for k := range f.Projects {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return f.Projects[keys[i]].Root < f.Projects[keys[j]].Root
		})
		for _, k := range keys {
			p := f.Projects[k]
			label := p.Root
			if label == "" {
				label = "(unknown)"
			}
			writeBlock(&b, label, p.Snapshot)
		}
	}
	return b.String()
}

func writeBlock(b *strings.Builder, label string, s Snapshot) {
	saved := Saved(s.BaselineChars, s.ActualChars)
	pct := Percent(s.BaselineChars, s.ActualChars)
	fmt.Fprintf(b, "  %s: ~%d tokens saved (%d%%) across %d run", label, saved, pct, s.Runs)
	if s.Runs != 1 {
		b.WriteByte('s')
	}
	b.WriteByte('\n')
}

// RunLine is the one-liner printed after a successful Gradle run.
func RunLine(baseline, actual int64) string {
	return fmt.Sprintf("gtv: saved ~%d tokens (%d%%) this run",
		Saved(baseline, actual), Percent(baseline, actual))
}

func filePath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gtv", "stats.json"), nil
}

func rootKey(root string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return hex.EncodeToString(sum[:])
}

func load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{Projects: map[string]Project{}}, nil
		}
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Projects == nil {
		f.Projects = map[string]Project{}
	}
	return &f, nil
}

func save(path string, f *File) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
