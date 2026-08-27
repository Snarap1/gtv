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

type Snapshot struct {
	BaselineChars int64 `json:"baseline_chars"`
	ActualChars   int64 `json:"actual_chars"`
	Runs          int64 `json:"runs"`
}

type Project struct {
	Root string `json:"root"`
	Snapshot
}

type File struct {
	Snapshot
	Projects map[string]Project `json:"projects"`
}

var (
	sessionMu sync.Mutex
	session   Snapshot
)

func Tokens(chars int64) int64 {
	if chars < 0 {
		return 0
	}
	return chars / 4
}

func Saved(baseline, actual int64) int64 {
	b, a := Tokens(baseline), Tokens(actual)
	if a >= b {
		return 0
	}
	return b - a
}

func Percent(baseline, actual int64) int {
	b := Tokens(baseline)
	if b == 0 {
		return 0
	}
	return int(Saved(baseline, actual) * 100 / b)
}

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

func Session() Snapshot {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	return session
}

func Load() (*File, error) {
	path, err := filePath()
	if err != nil {
		return nil, err
	}
	return load(path)
}

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
