package agents

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"tagent/internal/roles"
	"tagent/internal/tasks"

	"gopkg.in/yaml.v3"
)

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

var readOnlySources = []string{"global", "legacy"}

func readOnly(source string) bool { return slices.Contains(readOnlySources, source) }

// Save writes one agent: harness, model and effort go to its board.yaml role, and
// plain-markdown instructions go to .tasks/agents/<name>.md. The file is always
// resolved here from the repo's own view, never from the request, so a client
// cannot aim a write, and legacy harness directories are never touched.
func Save(dir, harness string, a Agent) error {
	if !nameRe.MatchString(a.Name) {
		return fmt.Errorf("agent name: start with a letter or digit, then letters, digits, - or _")
	}
	if !slices.Contains([]string{"pi", "claude", "codex"}, a.Harness) {
		return fmt.Errorf("unknown harness %q (pi, claude or codex)", a.Harness)
	}
	if err := tasks.ValidateEffort(a.Harness, a.Effort); err != nil {
		return err
	}
	if strings.ContainsAny(a.Model, "\r\n") {
		return fmt.Errorf("model must be one line")
	}
	if readOnly(a.Source) {
		return fmt.Errorf("%s agents are read-only; copy to the repo first", a.Source)
	}
	cur, found := Find(dir, harness, a.Name)
	if found && readOnly(cur.Source) {
		// "Copy to repo" asks for an app-owned file of the same name, which then
		// shadows the read-only one. Anything else would write into it.
		if a.Source != "repo" {
			return fmt.Errorf("%s agents are read-only; copy to the repo first", cur.Source)
		}
		found = false
	}
	path := ""
	if found {
		path = cur.Path
	}
	if a.Instructions != "" {
		path = roles.LocalPath(dir, a.Name)
	}
	if slices.Contains(boardNames(dir), a.Name) || a.Source == "board" || a.Model != "" || a.Effort != "" || a.Harness == "pi" || a.Instructions != "" {
		if err := editBoard(dir, func(rs *yaml.Node) error { return setRole(rs, a) }); err != nil {
			return err
		}
	}
	// Empty instructions never write: a client that saves only model/effort must
	// not create or wipe the file.
	if a.Instructions == "" {
		return nil
	}
	return writeInstructions(a.Instructions, path)
}

// Delete removes an agent's board role and its app-owned file. Read-only agents
// (global or legacy) are refused.
func Delete(dir, harness, name string) error {
	a, ok := Find(dir, harness, name)
	if !ok {
		return fmt.Errorf("no agent %q", name)
	}
	if readOnly(a.Source) {
		return fmt.Errorf("%s agents are read-only", a.Source)
	}
	if slices.Contains(boardNames(dir), name) {
		if err := editBoard(dir, func(rs *yaml.Node) error { removeKey(rs, name); return nil }); err != nil {
			return err
		}
	}
	path := roles.LocalPath(dir, name)
	if exists(path) {
		return os.Remove(path)
	}
	return nil
}

// writeInstructions replaces a file's plain markdown and does nothing when it is
// unchanged.
func writeInstructions(text, path string) error {
	data := []byte(strings.TrimRight(text, "\n") + "\n")
	old, err := os.ReadFile(path)
	if err == nil && bytes.Equal(old, data) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// Template is the starting file for a new agent: plain markdown, no frontmatter.
func Template(name string) string {
	return "# " + name + "\n\nDescribe how this agent works."
}

func boardNames(dir string) []string {
	cfg, _ := tasks.LoadBoardConfig(dir)
	var names []string
	for n := range cfg.Roles {
		names = append(names, n)
	}
	return names
}

// editBoard applies edit to the roles mapping of .tasks/board.yaml. It edits the yaml node
// tree so comments and key order survive, validates the result, then replaces the file.
// ponytail: last write wins if the TUI and web save at the same moment; add a file lock if that bites.
func editBoard(dir string, edit func(roles *yaml.Node) error) error {
	path := tasks.BoardConfigPath(dir)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return fmt.Errorf("no %s; run tagent board init first", path)
	}
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	top := doc.Content[0]
	rs := tasks.YAMLValue(top, "roles")
	if rs == nil {
		rs = &yaml.Node{Kind: yaml.MappingNode}
		top.Content = append(top.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "roles"}, rs)
	}
	if rs.Kind != yaml.MappingNode {
		*rs = yaml.Node{Kind: yaml.MappingNode}
	}
	if err := edit(rs); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if _, err := tasks.ParseBoardConfig(buf.Bytes()); err != nil {
		return err
	}
	return writeAtomic(path, buf.Bytes())
}

// setRole upserts a role's agent, model and effort. Other keys (fresh_after) stay as they are.
func setRole(rs *yaml.Node, a Agent) error {
	role := tasks.YAMLValue(rs, a.Name)
	if role == nil {
		role = &yaml.Node{Kind: yaml.MappingNode}
		rs.Content = append(rs.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: a.Name}, role)
	}
	if role.Kind != yaml.MappingNode {
		*role = yaml.Node{Kind: yaml.MappingNode}
	}
	tasks.SetYAMLField(role, "agent", a.Harness)
	tasks.SetYAMLField(role, "model", a.Model)
	tasks.SetYAMLField(role, "effort", a.Effort)
	return nil
}

func removeKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = slices.Delete(m.Content, i, i+2)
			return
		}
	}
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".board-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// EnsureFile creates the instruction file of an agent that has none (or copies a
// read-only one into the repo), so it can be edited.
func EnsureFile(dir, harness string, a Agent) (Agent, error) {
	if a.Path != "" && !readOnly(a.Source) {
		return a, nil
	}
	a.Source = "repo"
	if a.Instructions == "" {
		a.Instructions = Template(a.Name)
	}
	if err := Save(dir, harness, a); err != nil {
		return a, err
	}
	found, ok := Find(dir, harness, a.Name)
	if !ok {
		return a, fmt.Errorf("agent %q not found after save", a.Name)
	}
	return found, nil
}
