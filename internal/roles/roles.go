// Package roles discovers agent instructions. tagent owns the files: plain
// markdown in .tasks/agents (repo) and ~/.config/tagent/agents (global).
// Legacy harness directories (.claude/.codex/.opencode) are still read, but the
// app never writes there, and their harness is no longer implied by the process.
package roles

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"tagent/internal/apppath"

	"github.com/BurntSushi/toml"
)

// Definition is one instruction file. Harness is empty for app-owned files
// (the role's board.yaml entry or the process harness decides) and names the
// harness for legacy files.
type Definition struct {
	Name, Description, Instructions, Path, Harness string
	// Native means the file lives in the harness's own agents directory, so
	// that harness loads it itself and it must not be injected again.
	Native bool
}

// legacyHarnesses are the directories read for compatibility, in priority order.
var legacyHarnesses = []string{"claude", "codex", "opencode"}

// Harness is the kind of agent command runs ("claude", "pi", …); a name from
// harnesses.yml counts as the command it runs.
func Harness(command string) string {
	// ponytail: reads harnesses.yml per non-built-in lookup; cache if it shows in a profile.
	if command != "" && !slices.Contains(builtinHarnesses, command) {
		list, _ := LoadHarnesses()
		for _, h := range list {
			if h.Name == command && h.Command != "" {
				command = h.Command
			}
		}
	}
	return kindOf(command)
}

// kindOf is Harness without the harnesses.yml lookup, so LoadHarnesses can use it.
func kindOf(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "claude"
	}
	base := filepath.Base(fields[0])
	if base == "claude-api" { // second Claude Code install: same harness, own label
		return "claude"
	}
	return base
}

// roots caches git answers: a checkout's toplevel never changes, and the board asks per role on every refresh.
var roots sync.Map

func Root(dir string) string {
	if root, ok := roots.Load(dir); ok {
		return root.(string)
	}
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		root := strings.TrimSpace(string(out))
		roots.Store(dir, root)
		return root
	}
	return dir
}

// LocalDir is the app-owned instruction directory of a repo checkout.
func LocalDir(dir string) string {
	return filepath.Join(Root(dir), ".tasks", "agents")
}

// LocalPath is where the app keeps the instructions of agent name.
func LocalPath(dir, name string) string {
	return filepath.Join(LocalDir(dir), name+".md")
}

// MemoryPath is the per-agent memory file. It sits beside .tasks/agents, not in
// it, so ListLocal never lists it as a role.
func MemoryPath(dir, name string) string {
	return filepath.Join(Root(dir), ".tasks", "memory", name+".md")
}

// Memory is the trimmed memory of agent name, or "" when there is no file or the
// name would leave the memory directory.
func Memory(dir, name string) string {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return ""
	}
	b, err := os.ReadFile(MemoryPath(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// DirFor is a harness's own agents directory. It is read for legacy files and
// used when handing a native definition to a harness; the app never writes here.
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

// ListLocal is every instruction file in the repo: .tasks/agents first, then the
// legacy harness directories. It is not filtered by the process harness.
func ListLocal(dir string) []Definition {
	defs := listDir(LocalDir(dir), "", false)
	for _, h := range legacyHarnesses {
		defs = append(defs, listDir(DirFor(dir, h), h, true)...)
	}
	return sortUnique(defs)
}

func GlobalDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(apppath.Dir(base), "agents")
}

// ListGlobal is every global instruction file: ~/.config/tagent/agents/*.md
// first, then the legacy per-harness subdirectories.
func ListGlobal() []Definition {
	defs := listDir(GlobalDir(), "", false)
	for _, h := range legacyHarnesses {
		defs = append(defs, listDir(filepath.Join(GlobalDir(), h), h, false)...)
	}
	return sortUnique(defs)
}

// sortUnique drops later duplicates (so the first list wins) and sorts by name.
func sortUnique(defs []Definition) []Definition {
	seen := map[string]bool{}
	out := make([]Definition, 0, len(defs))
	for _, d := range defs {
		if seen[d.Name] {
			continue
		}
		seen[d.Name] = true
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func listDir(base, harness string, native bool) []Definition {
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
		d := Definition{Name: name, Path: path, Harness: harness, Native: native, Instructions: strings.TrimSpace(string(b))}
		switch harness {
		case "codex":
			var c struct {
				Name         string `toml:"name"`
				Description  string `toml:"description"`
				Instructions string `toml:"developer_instructions"`
			}
			if _, err := toml.Decode(string(b), &c); err != nil || c.Name == "" {
				return nil
			}
			d.Name, d.Description, d.Instructions = c.Name, c.Description, c.Instructions
		case "claude", "opencode":
			d.Description = frontmatter(d.Instructions, "description")
			if harness == "claude" {
				if n := frontmatter(d.Instructions, "name"); n != "" {
					d.Name = n
				}
			}
		default:
			// App-owned instructions are plain markdown; only tolerate a
			// frontmatter block, never hand it to a harness or the web UI.
			d.Description, d.Instructions = stripFrontmatter(d.Instructions)
		}
		defs = append(defs, d)
		return nil
	})
	return defs
}

// frontmatter returns one key of a leading --- block, "" when there is none.
func frontmatter(text, key string) string {
	if !strings.HasPrefix(text, "---\n") {
		return ""
	}
	front, _, ok := strings.Cut(text[4:], "\n---")
	if !ok {
		return ""
	}
	for _, line := range strings.Split(front, "\n") {
		k, value, found := strings.Cut(line, ":")
		if found && k == key {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return ""
}

// stripFrontmatter drops a leading --- block, returning its description and the
// plain markdown body.
func stripFrontmatter(text string) (description, body string) {
	if !strings.HasPrefix(text, "---\n") {
		return "", text
	}
	front, rest, ok := strings.Cut(text[4:], "\n---")
	if !ok {
		return "", text
	}
	for _, line := range strings.Split(front, "\n") {
		k, value, found := strings.Cut(line, ":")
		if found && k == "description" {
			description = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return description, strings.TrimSpace(strings.TrimPrefix(rest, "\n"))
}

func NamesIn(dir string) []string {
	var names []string
	for _, d := range ListLocal(dir) {
		names = append(names, d.Name)
	}
	return names
}

func GetIn(dir, name string) (Definition, bool) {
	for _, d := range append(ListLocal(dir), ListGlobal()...) {
		if d.Name == name {
			return d, true
		}
	}
	return Definition{}, false
}
