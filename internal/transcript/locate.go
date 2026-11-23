// Package transcript reads a session's conversation from the harness's own
// transcript files (claude, pi, codex). Discovery is bounded; parsing is per harness.
package transcript

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const discoveryEntries = 4096

var sessionPaths sync.Map

// Locate reads directory entries in batches, never an unbounded ReadDir/Glob.
// Exhaustion is distinct from missing storage: callers must fail closed.
func Locate(ctx context.Context, root, suffix string) (string, bool) {
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

// Find returns the transcript file of a session ("" when there is none).
func Find(ctx context.Context, harness, sessionID string) string {
	if sessionID == "" || strings.ContainsAny(sessionID, "/\\") {
		return ""
	}
	var root, suffix string
	switch harness {
	case "claude":
		root, suffix = ClaudeRoot(), string(filepath.Separator)+sessionID+".jsonl"
	case "codex":
		root, suffix = codexRoot(), sessionID+".jsonl"
	case "pi":
		home, _ := os.UserHomeDir()
		root, suffix = filepath.Join(home, ".pi", "agent", "sessions"), "_"+sessionID+".jsonl"
	default:
		return ""
	}
	path, _ := Locate(ctx, root, suffix)
	return path
}

// ClaudeRoot is where Claude Code keeps transcripts.
func ClaudeRoot() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "projects")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

func codexRoot() string {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".codex", "sessions")
	if d := os.Getenv("CODEX_HOME"); d != "" {
		root = filepath.Join(d, "sessions")
	}
	return root
}
