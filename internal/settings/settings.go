// Package settings is tagent's own config file: the source of truth for every
// user setting. The file is in tmux syntax (`set -g @key 'value'`), so the plugin
// can still source it, but tagent itself reads it directly: a setting never
// depends on a running tmux server, and tmux options are only a mirror.
package settings

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tagent/internal/apppath"
)

// Path is the config file.
func Path() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if testing.Testing() && (dir == "" || !strings.HasPrefix(dir, os.TempDir())) { // a test never reads the developer's real settings
		return filepath.Join(os.TempDir(), "tagent-test-no-config", "config.conf")
	}
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(apppath.Dir(dir), "config.conf")
}

var (
	mu    sync.Mutex
	cache map[string]string
	stamp time.Time
	where string
)

// All returns every setting in the file (key with its "@"). It re-reads the
// file only when its modification time changes.
func All() map[string]string {
	path := Path()
	info, err := os.Stat(path)
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		cache, where = nil, ""
		return nil
	}
	if cache != nil && where == path && info.ModTime().Equal(stamp) {
		return cache
	}
	b, _ := os.ReadFile(path)
	cache, where, stamp = parse(string(b)), path, info.ModTime()
	return cache
}

// Get reads one setting; ok is false when the file does not set it.
func Get(key string) (string, bool) {
	v, ok := All()[key]
	return v, ok
}

func parse(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || (f[0] != "set" && f[0] != "set-option") || f[1] != "-g" {
			continue
		}
		rest := strings.TrimSpace(line[strings.Index(line, f[2])+len(f[2]):])
		out[f[2]] = unquote(rest)
	}
	return out
}

// unquote reverses Quote and also accepts a bare or double-quoted word.
func unquote(v string) string {
	switch {
	case len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'':
		return strings.ReplaceAll(v[1:len(v)-1], `'\''`, "'")
	case len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"':
		return v[1 : len(v)-1]
	}
	return v
}

// Quote makes value one tmux word.
func Quote(v string) string { return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'" }
