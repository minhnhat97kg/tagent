package agents

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"tagent/internal/roles"
	"tagent/internal/tasks"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Save writes one agent: harness, model and effort go to its board.yaml role, and
// instructions go to its repo file. The file is always resolved here from the repo's own
// view (Find or the harness directory), never from the request, so a client cannot aim a write.
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
	if a.Harness == "pi" && a.Instructions != "" {
		return fmt.Errorf("pi agents have no instruction file")
	}
	if strings.ContainsAny(a.Model, "\r\n") {
		return fmt.Errorf("model must be one line")
	}
	if a.Source == "global" {
		return fmt.Errorf("global agents are read-only; copy to repo first")
	}
	cur, found := Find(dir, harness, a.Name)
	if found && cur.Source == "global" {
		// "Copy to repo" asks for a repo file of the same name, which then shadows the global one.
		if a.Source != "repo" {
			return fmt.Errorf("global agents are read-only; copy to repo first")
		}
		found = false
	}
	path := ""
	if found {
		path = cur.Path
	}
	if path != "" && a.Harness != "pi" && a.Instructions != "" && harnessOfPath(path) != a.Harness {
		return fmt.Errorf("%s is a %s file; its instructions can't move to %s here", a.Name, harnessOfPath(path), a.Harness)
	}
	if path == "" && a.Instructions != "" && a.Harness != "pi" {
		path = filepath.Join(roles.DirFor(dir, a.Harness), a.Name+ext(a.Harness))
		if exists(path) {
			return fmt.Errorf("%s already exists", path)
		}
	}
	if slices.Contains(boardNames(dir), a.Name) || a.Model != "" || a.Effort != "" || a.Harness == "pi" {
		if err := editBoard(dir, func(rs *yaml.Node) error { return setRole(rs, a) }); err != nil {
			return err
		}
	}
	// Empty instructions never write: a client that saves only model/effort must not wipe the file.
	if a.Harness == "pi" || a.Instructions == "" {
		return nil
	}
	return writeInstructions(a, path)
}

// Delete removes an agent's board role and its repo file. Global agents are refused.
func Delete(dir, harness, name string) error {
	a, ok := Find(dir, harness, name)
	if !ok {
		return fmt.Errorf("no agent %q", name)
	}
	if a.Source == "global" {
		return fmt.Errorf("global agents are read-only")
	}
	if slices.Contains(boardNames(dir), name) {
		if err := editBoard(dir, func(rs *yaml.Node) error { removeKey(rs, name); return nil }); err != nil {
			return err
		}
	}
	if a.Source == "repo" && a.Path != "" {
		return os.Remove(a.Path)
	}
	return nil
}

func ext(harness string) string {
	if harness == "codex" {
		return ".toml"
	}
	return ".md"
}

func harnessOfPath(path string) string {
	if strings.HasSuffix(path, ".toml") {
		return "codex"
	}
	return "claude"
}

// writeInstructions replaces a file's instructions, and does nothing when they are unchanged.
// Claude files are the whole markdown (frontmatter included); codex files keep everything
// else, comments included, and only developer_instructions changes.
func writeInstructions(a Agent, path string) error {
	old, err := os.ReadFile(path)
	creating := os.IsNotExist(err)
	if err != nil && !creating {
		return err
	}
	data := []byte(a.Instructions + "\n")
	if a.Harness == "codex" {
		if data, err = codexWithInstructions(old, creating, a, path); err != nil {
			return err
		}
	}
	if !creating && bytes.Equal(old, data) {
		return nil
	}
	if creating {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	}
	return os.WriteFile(path, data, 0644)
}

var devInstructionsRe = regexp.MustCompile(`(?m)^developer_instructions\s*=`)

// codexWithInstructions returns the codex file with developer_instructions set. It splices the
// one line into the original text and keeps the result only if it decodes to the old keys plus
// the new value, so comments and layout outside that line are never rewritten.
func codexWithInstructions(old []byte, creating bool, a Agent, path string) ([]byte, error) {
	if creating {
		var buf bytes.Buffer
		err := toml.NewEncoder(&buf).Encode(map[string]string{"name": a.Name, "developer_instructions": a.Instructions})
		return buf.Bytes(), err
	}
	var cur map[string]any
	if _, err := toml.Decode(string(old), &cur); err != nil {
		return nil, err
	}
	if s, _ := cur["developer_instructions"].(string); s == a.Instructions {
		return old, nil
	}
	var lit bytes.Buffer
	if err := toml.NewEncoder(&lit).Encode(map[string]string{"developer_instructions": a.Instructions}); err != nil {
		return nil, err
	}
	line := strings.TrimSuffix(lit.String(), "\n")
	want := maps.Clone(cur)
	want["developer_instructions"] = a.Instructions
	s := string(old)
	var candidates []string
	if loc := devInstructionsRe.FindStringIndex(s); loc != nil {
		// Try each line end as the end of the old statement; the first that decodes right wins.
		for i := loc[0]; i <= len(s); i++ {
			if i == len(s) || s[i] == '\n' {
				candidates = append(candidates, s[:loc[0]]+line+s[i:])
			}
		}
	} else {
		at := len(s)
		if i := strings.Index(s, "\n["); i >= 0 {
			at = i + 1
		} else if strings.HasPrefix(s, "[") {
			at = 0
		}
		pre := ""
		if at > 0 && s[at-1] != '\n' {
			pre = "\n"
		}
		candidates = []string{s[:at] + pre + line + "\n" + s[at:]}
	}
	for _, c := range candidates {
		var got map[string]any
		if _, err := toml.Decode(c, &got); err == nil && reflect.DeepEqual(got, want) {
			return []byte(c), nil
		}
	}
	return nil, fmt.Errorf("%s: can't set developer_instructions without changing other keys", path)
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// Template is the starting file for a new claude agent.
func Template(name string) string {
	return "---\nname: " + name + "\ndescription: \n---\n\nDescribe how this agent works.\n"
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
	rs := valueOf(top, "roles")
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
	role := valueOf(rs, a.Name)
	if role == nil {
		role = &yaml.Node{Kind: yaml.MappingNode}
		rs.Content = append(rs.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: a.Name}, role)
	}
	if role.Kind != yaml.MappingNode {
		*role = yaml.Node{Kind: yaml.MappingNode}
	}
	setField(role, "agent", a.Harness)
	setField(role, "model", a.Model)
	setField(role, "effort", a.Effort)
	return nil
}

// setField sets a scalar key; an empty value removes the key.
func setField(m *yaml.Node, key, value string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			continue
		}
		if value == "" {
			m.Content = slices.Delete(m.Content, i, i+2)
		} else {
			m.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Value: value}
		}
		return
	}
	if value != "" {
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Value: value})
	}
}

func removeKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = slices.Delete(m.Content, i, i+2)
			return
		}
	}
}

func valueOf(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
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

// EnsureFile creates the instruction file of an agent that has none, so it can be edited.
func EnsureFile(dir, harness string, a Agent) (Agent, error) {
	if a.Path != "" {
		return a, nil
	}
	if a.Harness == "pi" {
		return a, fmt.Errorf("pi agents have no instruction file")
	}
	a.Instructions = "Describe how this agent works."
	if a.Harness == "claude" {
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
