package watch

import (
	"io/fs"
	"path/filepath"
	"time"
)

const pollInterval = 500 * time.Millisecond

var skipDir = map[string]bool{
	".git": true, ".gradle": true, ".idea": true, "build": true, "out": true, "node_modules": true,
}

func Until(dir string, run func()) {
	last := snapshot(dir)
	run()
	for {
		time.Sleep(pollInterval)
		if cur := snapshot(dir); !sameSnapshot(cur, last) {
			last = cur
			run()
		}
	}
}

type fileStamp struct {
	mtime int64
	size  int64
}

func snapshot(dir string) map[string]fileStamp {
	entries := make(map[string]fileStamp)
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && path != dir && skipDir[d.Name()] {
			return fs.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		entries[path] = fileStamp{mtime: info.ModTime().UnixNano(), size: info.Size()}
		return nil
	})
	return entries
}

func sameSnapshot(a, b map[string]fileStamp) bool {
	if len(a) != len(b) {
		return false
	}
	for path, stamp := range a {
		if b[path] != stamp {
			return false
		}
	}
	return true
}
