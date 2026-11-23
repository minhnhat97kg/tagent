package tasks

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// BoardExample is the starter config used by board init and the dashboard editor.
const BoardExample = `roles:
  planner: {agent: pi, model: openai-codex/gpt-6.1-sol, effort: high}
  implementer: {agent: pi, model: openai-codex/gpt-6.1-sol, fresh_after: 3}
  reviewer: {agent: claude}
statuses:
  - {id: todo, next: [plan]}
  - {id: plan, role: planner, next: [build, blocked]}
  - {id: build, role: implementer, next: [test, blocked]}
  - {id: test, run: "go test ./...", pass: review, fail: build}
  - {id: review, role: reviewer, next: [done, build, blocked], max_loops: 3}
  - {id: blocked}
  - {id: done}
max_fires_per_hour: 20
triggers:
  - on: mention
    when: {to: '*'}
    agent: '{{event.name}}'
    prompt: "{{event.actor}} mentioned you on task {{task.id}}: {{comment.body}}. Re-read the ticket and reply with a comment."
`

type Filter struct {
	To    string `yaml:"to"`
	From  string `yaml:"from"`
	Actor string `yaml:"actor"`
}
type Trigger struct {
	On     string `yaml:"on"`
	When   Filter `yaml:"when"`
	Agent  string `yaml:"agent"`
	Prompt string `yaml:"prompt"`
}
type Role struct {
	FreshAfter *int   `yaml:"fresh_after,omitempty"`
	Effort     string `yaml:"effort,omitempty"`
	Agent      string `yaml:"agent"`
	Model      string `yaml:"model"`
}
type Status struct {
	FreshAfter *int     `yaml:"fresh_after,omitempty"`
	Needs      []string `yaml:"-"`
	Failure    string   `yaml:"-"`
	Prompt     string   `yaml:"prompt,omitempty"`
	Model      string   `yaml:"model,omitempty"`
	Job        string   `yaml:"-"`
	Outputs    []string `yaml:"-"`
	ID         string   `yaml:"id"`
	Role       string   `yaml:"role"`
	Next       []string `yaml:"next"`
	Run        string   `yaml:"run"`
	Pass       string   `yaml:"pass"`
	Fail       string   `yaml:"fail"`
	MaxLoops   int      `yaml:"max_loops"`
	Column     string   `yaml:"column"` // board column; default: todo, done, else "in progress"
}
type BoardConfig struct {
	Roles           map[string]Role `yaml:"roles"`
	Statuses        []Status        `yaml:"statuses"`
	Columns         []string        `yaml:"columns"`
	Triggers        []Trigger       `yaml:"triggers"`
	MaxFiresPerHour int             `yaml:"max_fires_per_hour"`
}

func DefaultColumns() []string { return []string{"todo", "doing", "review", "blocked", "done"} }

// MainRoot deliberately uses the common git dir, so linked worktrees share config.
func MainRoot(dir string) string {
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output(); err == nil {
		return filepath.Dir(strings.TrimSpace(string(out)))
	}
	abs, err := filepath.Abs(dir)
	if err == nil {
		return abs
	}
	return dir
}
func BoardConfigPath(dir string) string { return filepath.Join(MainRoot(dir), ".tasks", "board.yaml") }
func ParseBoardConfig(b []byte) (BoardConfig, error) {
	c := BoardConfig{MaxFiresPerHour: 20}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(&c); err != nil && err != io.EOF {
		return c, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("expected one YAML document")
	}
	if c.Columns != nil && c.Statuses != nil {
		return c, fmt.Errorf("use statuses or columns, not both")
	}
	if c.Statuses == nil {
		if c.Columns == nil {
			c.Columns = DefaultColumns()
		}
		for _, id := range c.Columns {
			c.Statuses = append(c.Statuses, Status{ID: id})
		}
	} else {
		for _, s := range c.Statuses {
			c.Columns = append(c.Columns, s.ID)
		}
	}
	seen := map[string]bool{}
	for _, s := range c.Columns {
		if strings.TrimSpace(s) == "" || seen[s] {
			return c, fmt.Errorf("invalid or duplicate column %q", s)
		}
		seen[s] = true
	}
	if !seen["todo"] || !seen["done"] {
		return c, fmt.Errorf("columns must include todo and done")
	}
	for name, r := range c.Roles {
		if err := ValidateEffort(r.Agent, r.Effort); err != nil {
			return c, fmt.Errorf("role %s: %w", name, err)
		}
		if strings.TrimSpace(name) == "" || strings.TrimSpace(r.Agent) == "" || (r.FreshAfter != nil && *r.FreshAfter < 0) {
			return c, fmt.Errorf("role requires name and agent")
		}
	}
	for _, s := range c.Statuses {
		if s.MaxLoops < 0 || (s.FreshAfter != nil && *s.FreshAfter < 0) || (s.Role != "" && s.Run != "") {
			return c, fmt.Errorf("invalid status %s", s.ID)
		}
		if (s.Role != "" || s.MaxLoops > 0) && !seen["blocked"] {
			return c, fmt.Errorf("status %s requires blocked status", s.ID)
		}
		if s.Run != "" && (s.Pass == "" || s.Fail == "") {
			return c, fmt.Errorf("run status %s requires pass and fail", s.ID)
		}
		if s.Run == "" && (s.Pass != "" || s.Fail != "") {
			return c, fmt.Errorf("pass/fail require run")
		}
		nextSeen := map[string]bool{}
		for _, id := range s.Next {
			if !seen[id] || nextSeen[id] {
				return c, fmt.Errorf("invalid next %q", id)
			}
			nextSeen[id] = true
		}
		for _, id := range []string{s.Pass, s.Fail} {
			if id != "" && !seen[id] {
				return c, fmt.Errorf("unknown status %q", id)
			}
		}
	}
	if c.MaxFiresPerHour < 1 {
		return c, fmt.Errorf("max_fires_per_hour must be positive")
	}
	for _, t := range c.Triggers {
		if !slices.Contains([]string{"create", "assign", "status", "comment", "mention", "due"}, t.On) {
			return c, fmt.Errorf("invalid trigger kind %q", t.On)
		}
		if strings.TrimSpace(t.Agent) == "" {
			return c, fmt.Errorf("trigger requires agent; replace workflow targets with statuses")
		}
		for _, s := range []string{t.Agent, t.Prompt} {
			if err := ValidateTemplate(s); err != nil {
				return c, err
			}
		}
	}
	return c, nil
}
func ValidateTemplate(s string) error {
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			break
		}
		s = s[i+2:]
		j := strings.Index(s, "}}")
		if j < 0 {
			return fmt.Errorf("unterminated template")
		}
		key := strings.TrimSpace(s[:j])
		switch key {
		case "task.id", "task.title", "task.text", "task.status", "task.due", "task.assignees", "event.kind", "event.actor", "event.from", "event.to", "event.name", "comment.body", "comment.author":
		default:
			return fmt.Errorf("unknown template variable %q", key)
		}
		s = s[j+2:]
	}
	return nil
}
func LoadBoardConfig(dir string) (BoardConfig, error) {
	b, err := os.ReadFile(BoardConfigPath(dir))
	if os.IsNotExist(err) {
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			h, _ := os.UserHomeDir()
			base = filepath.Join(h, ".config")
		}
		b, err = os.ReadFile(filepath.Join(base, "tmux-agent", "board.yaml"))
	}
	if os.IsNotExist(err) {
		return ParseBoardConfig(nil)
	}
	if err != nil {
		return BoardConfig{}, err
	}
	return ParseBoardConfig(b)
}
func SaveBoardConfig(dir string, b []byte) error {
	if _, err := ParseBoardConfig(b); err != nil {
		return err
	}
	p := BoardConfigPath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return AtomicWrite(p, b)
}
func AtomicWrite(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Lane is the board column a status is shown in: many statuses, few columns.
func (c BoardConfig) Lane(status string) string {
	for _, s := range c.Statuses {
		if s.ID == status && s.Column != "" {
			return s.Column
		}
	}
	switch status {
	case "todo", "done":
		return status
	}
	return "in progress"
}

// Lanes lists the board columns in status order.
func (c BoardConfig) Lanes() []string {
	var out []string
	for _, id := range c.Columns {
		if l := c.Lane(id); !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}
