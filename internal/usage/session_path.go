package usage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const replyBytes = 4 << 20
const replyRecords = 4096
const discoveryEntries = 4096

var sessionPaths sync.Map

// findSession reads directory entries in batches, never an unbounded ReadDir/Glob.
// Exhaustion is distinct from missing storage: callers must fail closed.
func findSession(ctx context.Context, root, suffix string) (string, bool) {
	key := root + "\x00" + suffix
	if p, ok := sessionPaths.Load(key); ok {
		return p.(string), false
	}
	remaining := discoveryEntries
	var walk func(string, int) (string, bool)
	walk = func(dir string, depth int) (string, bool) {
		if ctx.Err() != nil || remaining <= 0 || depth > 8 {
			return "", true
		}
		f, err := os.Open(dir)
		if err != nil {
			return "", false
		}
		defer f.Close()
		for {
			if ctx.Err() != nil || remaining <= 0 {
				return "", true
			}
			entries, err := f.ReadDir(64)
			for _, e := range entries {
				remaining--
				if remaining < 0 || ctx.Err() != nil {
					return "", true
				}
				p := filepath.Join(dir, e.Name())
				if e.IsDir() {
					if found, exhausted := walk(p, depth+1); found != "" || exhausted {
						return found, exhausted
					}
				} else if e.Type().IsRegular() && strings.HasSuffix(p, suffix) {
					return p, false
				}
			}
			if err != nil {
				return "", false
			}
		}
	}
	path, exhausted := walk(root, 0)
	if path != "" {
		sessionPaths.Store(key, path)
	}
	return path, exhausted
}
