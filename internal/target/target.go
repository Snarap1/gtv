// Package target turns a short argument like "UserServiceTest" or
// "UserServiceTest.should pass" into a Gradle task path and, when
// needed, a --tests filter — so callers can type a class name instead of the
// full `:module:test` task.
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

// Target is a resolved Gradle invocation.
type Target struct {
	Task       string // e.g. ":a:b:test"
	TestFilter string // e.g. "com.example.FooTest.should pass", empty for a bare task arg
}

// ErrAmbiguous means the argument matched more than one class; ErrNotFound
// means it matched none. Both are wrapped so callers can distinguish them
// with errors.Is.
var (
	ErrAmbiguous = errors.New("ambiguous target")
	ErrNotFound  = errors.New("target not found")
)

// Candidate is one match offered back to the caller when an argument is
// ambiguous.
type Candidate struct {
	FQN  string
	File string
}

// Resolve turns arg into a Target. root is the Gradle project root (as found
// by runner.FindGradleRoot). reindex forces a full rebuild of the class index
// instead of trusting the cache.
//
// arg forms, tried in this order:
//   - ":a:b:test"                      -> used as-is, no --tests filter
//   - a path to a .kt/.java file        -> FQN from its package + file name
//   - a simple class name or FQN        -> looked up under **/src/test/**
//   - "Class.method" or "Class::method" -> --tests "FQN.method"
//
// On ErrAmbiguous, the returned candidates list every match; the caller
// should print them and exit rather than guess.
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

type methodTarget struct{ class, method string }

// splitMethods returns possible class/method boundaries. :: is unambiguous;
// with '.', try from right to left so a package-qualified class and a method
// name that itself contains dots both resolve correctly.
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

// resolveSourcePath tries the argument as given, then relative to the
// working directory, then relative to the project root.
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

// fqnForFile derives the fully-qualified class name from the package
// declaration and the file name — not from parsing class bodies, which would
// need a real Kotlin/Java parser for nested and multi-class files.
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

// taskForFile walks up from the file to the nearest build.gradle(.kts),
// stopping at root, and turns the module's path relative to root into a
// Gradle task path.
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

// Index is the cached set of test classes found under root.
type Index struct {
	Root string `json:"root"`
	// Dirs maps every directory under a src/test tree to the mtime it had
	// when the index was built. A changed or new mtime anywhere in this set
	// (a file added, removed, or renamed) invalidates the cache: creating,
	// removing, or renaming an entry always touches its parent directory's
	// mtime, so this catches additions without re-scanning file contents.
	Dirs map[string]int64 `json:"dirs"`
	// Files records every indexed source file's mtime. Editing a file leaves
	// its parent directory's mtime unchanged, so Dirs alone cannot invalidate
	// a cached FQN after a package or class rename.
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

// buildIndex walks root once, collecting every .kt/.java file under a
// **/src/test/** tree and the mtime of every directory in that tree.
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

// loadOrBuildIndex trusts the cache when its recorded directory mtimes still
// match disk; otherwise, or when the cache directory itself is unavailable,
// it rebuilds and best-effort refreshes the cache. A cache write failure must
// not fail resolution — the index still works for this run.
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
	// Older cache files have no file mtimes, so rebuild them once rather than
	// silently retaining their old invalidation semantics.
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
