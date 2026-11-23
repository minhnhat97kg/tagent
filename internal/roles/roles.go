// Package roles discovers repository agents in the selected harness's native format.
package roles

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type Definition struct{ Name, Description, Instructions, Path, Harness string }

func Harness(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "claude"
	}
	return filepath.Base(fields[0])
}

func Root(dir string) string {
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return dir
}

func DirFor(dir, harness string) string {
	switch Harness(harness) {
	case "claude":
		return filepath.Join(Root(dir), ".claude", "agents")
	case "codex":
		return filepath.Join(Root(dir), ".codex", "agents")
	case "opencode":
		return filepath.Join(Root(dir), ".opencode", "agents")
	}
	return ""
}

func ListIn(dir, harness string) []Definition {
	harness = Harness(harness)
	return listDir(DirFor(dir, harness), harness)
}

func GlobalDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "tagent", "agents")
}
func ListGlobal(harness string) []Definition {
	defs := listDir(filepath.Join(GlobalDir(), Harness(harness)), Harness(harness))
	files, _ := filepath.Glob(filepath.Join(GlobalDir(), "*.md"))
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".md")
		if d, ok := Universal(name); ok {
			defs = append(defs, d)
		}
	}
	return defs
}

func listDir(base, harness string) []Definition {
	if base == "" {
		return nil
	}
	ext := ".md"
	if harness == "codex" {
		ext = ".toml"
	}
	var defs []Definition
	filepath.WalkDir(base, func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() || filepath.Ext(path) != ext {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return nil
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ext)
		if harness != "opencode" && strings.Contains(name, "/") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		d := Definition{Name: name, Path: path, Harness: harness, Instructions: strings.TrimSpace(string(b))}
		if harness == "codex" {
			var c struct {
				Name         string `toml:"name"`
				Description  string `toml:"description"`
				Instructions string `toml:"developer_instructions"`
			}
			if _, err := toml.Decode(string(b), &c); err != nil || c.Name == "" {
				return nil
			}
			d.Name, d.Description, d.Instructions = c.Name, c.Description, c.Instructions
		} else if strings.HasPrefix(d.Instructions, "---\n") {
			if front, _, ok := strings.Cut(d.Instructions[4:], "\n---"); ok {
				for _, line := range strings.Split(front, "\n") {
					key, value, found := strings.Cut(line, ":")
					if !found {
						continue
					}
					value = strings.Trim(strings.TrimSpace(value), `"'`)
					if key == "description" {
						d.Description = value
					}
					if key == "name" && harness == "claude" && value != "" {
						d.Name = value
					}
				}
			}
		}
		defs = append(defs, d)
		return nil
	})
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

func NamesIn(dir, harness string) []string {
	var names []string
	for _, d := range ListIn(dir, harness) {
		names = append(names, d.Name)
	}
	return names
}

func GetIn(dir, harness, name string) (Definition, bool) {
	for _, d := range append(ListIn(dir, harness), ListGlobal(harness)...) {
		if d.Name == name {
			return d, true
		}
	}
	return Universal(name)
}
