package library

import (
	"os"
	"path/filepath"
	"strings"
)

// ListNative returns installed harness skills without granting tagent permission to edit them.
// Library entries take precedence over native entries with the same name.
func ListNative(home, root string) []Skill {
	var out []Skill
	seen := map[string]bool{}
	add := func(dir, source string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.Type()&os.ModeSymlink != 0 {
				if resolved, err := filepath.EvalSymlinks(p); err == nil {
					p = resolved
				}
			}
			info, err := os.Stat(p)
			if err != nil || !info.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(p, "SKILL.md")); err != nil {
				continue
			}
			name := e.Name()
			if strings.TrimSpace(name) == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, Skill{Name: name, Path: p, Source: source})
		}
	}
	add(filepath.Join(home, ".claude", "skills"), "claude-user")
	add(filepath.Join(root, ".claude", "skills"), "claude-project")
	add(filepath.Join(home, ".codex", "skills"), "codex")
	add(filepath.Join(home, ".pi", "agent", "skills"), "pi")
	return out
}
