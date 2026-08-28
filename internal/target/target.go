package target

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Target struct {
	Task       string
	TestFilter string
}

var (
	ErrAmbiguous = errors.New("ambiguous target")
	ErrNotFound  = errors.New("target not found")
)

type Candidate struct {
	FQN  string
	File string
}

func Resolve(root, arg string, reindex bool) (Target, []Candidate, error) {
	if strings.HasPrefix(arg, ":") {
		return Target{Task: arg}, nil, nil
	}

	task, fqn, cands, err := resolveClass(root, arg, reindex)
	if err == nil {
		return Target{Task: task, TestFilter: fqn}, nil, nil
	}
	if errors.Is(err, ErrNotFound) {
		for _, candidate := range splitMethods(arg) {
			task, fqn, cands, err = resolveClass(root, candidate.class, reindex)
			if err == nil {
				return Target{Task: task, TestFilter: fqn + "." + candidate.method}, nil, nil
			}
			if !errors.Is(err, ErrNotFound) {
				return Target{}, cands, err
			}
		}
	}
	return Target{}, cands, err
}

// ResolveModule resolves arg to a Gradle module path (":a:b", "" for the root
// project) rather than a test task. Unlike Resolve, it does not try
// Class.method splitting: a compile/build target names a module, class, FQN,
// or source file, never a test method.
func ResolveModule(root, arg string, reindex bool) (string, []Candidate, error) {
	if strings.HasPrefix(arg, ":") {
		return strings.TrimSuffix(arg, ":test"), nil, nil
	}
	task, _, cands, err := resolveClass(root, arg, reindex)
	if err != nil {
		return "", cands, err
	}
	return strings.TrimSuffix(task, ":test"), nil, nil
}

type methodTarget struct{ class, method string }

func splitMethods(arg string) []methodTarget {
	if i := strings.Index(arg, "::"); i >= 0 {
		return []methodTarget{{class: arg[:i], method: arg[i+2:]}}
	}
	var out []methodTarget
	for i := strings.LastIndex(arg, "."); i >= 0; i = strings.LastIndex(arg[:i], ".") {
		out = append(out, methodTarget{class: arg[:i], method: arg[i+1:]})
	}
	return out
}

func resolveClass(root, class string, reindex bool) (task, fqn string, cands []Candidate, err error) {
	if looksLikeSourcePath(class) {
		abs, ferr := resolveSourcePath(root, class)
		if ferr != nil {
			return "", "", nil, ferr
		}
		fqn, ferr = fqnForFile(abs)
		if ferr != nil {
			return "", "", nil, ferr
		}
		task, ferr = taskForFile(root, abs)
		if ferr != nil {
			return "", "", nil, ferr
		}
		return task, fqn, nil, nil
	}

	idx, ierr := loadOrBuildIndex(root, reindex)
	if ierr != nil {
		return "", "", nil, ierr
	}
	matches := idx.find(class)
	switch len(matches) {
	case 0:
		return "", "", nil, fmt.Errorf("%w: %q", ErrNotFound, class)
	case 1:
		return matches[0].Task, matches[0].FQN, nil, nil
	default:
		cands = make([]Candidate, len(matches))
		for i, m := range matches {
			cands[i] = Candidate{FQN: m.FQN, File: m.File}
		}
		return "", "", cands, fmt.Errorf("%w: %q", ErrAmbiguous, class)
	}
}

func looksLikeSourcePath(s string) bool {
	return strings.HasSuffix(s, ".kt") || strings.HasSuffix(s, ".java")
}

func resolveSourcePath(root, p string) (string, error) {
	candidates := []string{p}
	if !filepath.IsAbs(p) {
		if cwd, err := os.Getwd(); err == nil {
			candidates = append(candidates, filepath.Join(cwd, p))
		}
		candidates = append(candidates, filepath.Join(root, p))
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return filepath.Abs(c)
		}
	}
	return "", fmt.Errorf("%w: no such file %q", ErrNotFound, p)
}

var packageDecl = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+)\s*;?\s*$`)

func fqnForFile(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	name := filepath.Base(file)
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".kt"), ".java")
	if m := packageDecl.FindSubmatch(data); m != nil {
		return string(m[1]) + "." + name, nil
	}
	return name, nil
}

func taskForFile(root, file string) (string, error) {
	root = filepath.Clean(root)
	moduleDir := root
	for d := filepath.Dir(file); ; {
		if hasBuildFile(d) {
			moduleDir = d
			break
		}
		if d == root {
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}

	rel, err := filepath.Rel(root, moduleDir)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return ":test", nil
	}
	return ":" + strings.Join(strings.Split(filepath.ToSlash(rel), "/"), ":") + ":test", nil
}

func hasBuildFile(dir string) bool {
	for _, name := range [...]string{"build.gradle.kts", "build.gradle"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

type Index struct {
	Root string `json:"root"`

	Dirs map[string]int64 `json:"dirs"`

	Files   map[string]int64 `json:"files"`
	Classes []classEntry     `json:"classes"`
}

type classEntry struct {
	FQN        string `json:"fqn"`
	SimpleName string `json:"simple"`
	File       string `json:"file"`
	Task       string `json:"task"`
}

func (idx *Index) find(class string) []classEntry {
	var out []classEntry
	byFQN := strings.Contains(class, ".")
	for _, c := range idx.Classes {
		if byFQN {
			if c.FQN == class {
				out = append(out, c)
			}
		} else if c.SimpleName == class {
			out = append(out, c)
		}
	}
	return out
}

var skipDir = map[string]bool{
	".git": true, ".gradle": true, ".idea": true, "build": true, "out": true, "node_modules": true,
}

func buildIndex(root string) (*Index, error) {
	idx := &Index{Root: filepath.Clean(root), Dirs: map[string]int64{}, Files: map[string]int64{}}

	var testRoots []string
	underTestRoot := func(p string) bool {
		for _, tr := range testRoots {
			if p == tr || strings.HasPrefix(p, tr+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}

	walkErr := filepath.WalkDir(idx.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != idx.Root && skipDir[d.Name()] {
				return fs.SkipDir
			}
			if d.Name() == "test" && filepath.Base(filepath.Dir(path)) == "src" {
				testRoots = append(testRoots, path)
			}
			if underTestRoot(path) {
				info, err := d.Info()
				if err != nil {
					return err
				}
				idx.Dirs[path] = info.ModTime().UnixNano()
			}
			return nil
		}
		if !underTestRoot(path) {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".kt" && ext != ".java" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		idx.Files[path] = info.ModTime().UnixNano()
		fqn, err := fqnForFile(path)
		if err != nil {
			return err
		}
		task, err := taskForFile(idx.Root, path)
		if err != nil {
			return err
		}
		simple := fqn
		if i := strings.LastIndex(fqn, "."); i >= 0 {
			simple = fqn[i+1:]
		}
		idx.Classes = append(idx.Classes, classEntry{FQN: fqn, SimpleName: simple, File: path, Task: task})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(idx.Classes, func(i, j int) bool { return idx.Classes[i].FQN < idx.Classes[j].FQN })
	return idx, nil
}

func cachePath(root string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.Clean(root)))
	return filepath.Join(base, "gtv", hex.EncodeToString(sum[:])+".json"), nil
}

func loadOrBuildIndex(root string, reindex bool) (*Index, error) {
	path, err := cachePath(root)
	if err != nil {
		return buildIndex(root)
	}
	if !reindex {
		if idx, ok := loadCached(path, root); ok {
			return idx, nil
		}
	}
	idx, err := buildIndex(root)
	if err != nil {
		return nil, err
	}
	if data, err := json.Marshal(idx); err == nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			_ = os.WriteFile(path, data, 0o644)
		}
	}
	return idx, nil
}

func loadCached(path, root string) (*Index, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var idx Index
	if json.Unmarshal(data, &idx) != nil {
		return nil, false
	}
	if idx.Root != filepath.Clean(root) {
		return nil, false
	}

	if idx.Dirs == nil || idx.Files == nil {
		return nil, false
	}
	for dir, mtime := range idx.Dirs {
		info, err := os.Stat(dir)
		if err != nil || info.ModTime().UnixNano() != mtime {
			return nil, false
		}
	}
	for file, mtime := range idx.Files {
		info, err := os.Stat(file)
		if err != nil || info.ModTime().UnixNano() != mtime {
			return nil, false
		}
	}
	return &idx, true
}
