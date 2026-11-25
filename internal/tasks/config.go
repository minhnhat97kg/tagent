package tasks

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"tagent/internal/apppath"

	"gopkg.in/yaml.v3"
)

// BoardExample is the starter config used by board init and the dashboard editor.
// Workflows hold the flow; board.yaml holds roles, triggers and the default workflow.
const BoardExample = `roles:
  planner: {agent: pi, model: openai-codex/gpt-6.1-sol, effort: high}
  implementer: {agent: pi, model: openai-codex/gpt-6.1-sol, fresh_after: 3}
  reviewer: {agent: claude}
default_workflow: feature
max_fires_per_hour: 20
triggers:
  - on: mention
    when: {to: '*'}
    agent: '{{event.name}}'
    prompt: "{{event.actor}} mentioned you on task {{task.id}}. Reply with tagent task comment {{task.id}} <text>."
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
	Agent      string `yaml:"agent,omitempty"`
	Model      string `yaml:"model"`
	Tier       string `yaml:"tier,omitempty"` // harness and model come from routing.yaml at dispatch
	// LongLived keeps one standing pane for the role across tickets (no ticket in its key), never refreshed by default.
	LongLived   bool `yaml:"long_lived,omitempty"`
	RoleContext `yaml:",inline"`
}
type Status struct {
	FreshAfter *int     `yaml:"fresh_after,omitempty"`
	Needs      []string `yaml:"-"`
	Failure    string   `yaml:"-"`
	Prompt     string   `yaml:"prompt,omitempty"`
	Model      string   `yaml:"model,omitempty"`
	Job        string   `yaml:"-"`
	Uses       string   `yaml:"-"`
	Outputs    []string `yaml:"-"`
	ID         string   `yaml:"id"`
	Role       string   `yaml:"role"`
	Next       []string `yaml:"next"`
	Run        string   `yaml:"run"`
	Pass       string   `yaml:"pass"`
	Fail       string   `yaml:"fail"`
	MaxLoops   int      `yaml:"max_loops"`
	Column     string   `yaml:"column,omitempty"` // deprecated: board lanes are fixed
}
type BoardConfig struct {
	Roles           map[string]Role `yaml:"roles"`
	Statuses        []Status        `yaml:"statuses,omitempty"` // deprecated input: converted to the implicit "board" workflow
	Columns         []string        `yaml:"columns,omitempty"`  // deprecated input
	DefaultWorkflow string          `yaml:"default_workflow,omitempty"`
	// LaneStatuses declares which statuses sit in which board lane (todo,
	// in progress, blocked, done); every ticket may be moved to them.
	LaneStatuses    map[string][]string `yaml:"lanes,omitempty"`
	Triggers        []Trigger           `yaml:"triggers"`
	MaxFiresPerHour int                 `yaml:"max_fires_per_hour"`
	// Pricing adds or overrides a model's API list price (USD per MTok) for the usage cost estimate.
	Pricing map[string]ModelPrice `yaml:"pricing,omitempty"`

	Legacy              bool     `yaml:"-"` // statuses were converted to the implicit workflow "board"
	Warnings            []string `yaml:"-"`
	defaultWorkflowLine int      `yaml:"-"`
}

// ModelPrice is USD per million tokens, the same four rates an API bill has.
type ModelPrice struct {
	Input      float64 `yaml:"input" json:"input"`
	Output     float64 `yaml:"output" json:"output"`
	CacheRead  float64 `yaml:"cache_read" json:"cache_read"`
	CacheWrite float64 `yaml:"cache_write" json:"cache_write"`
}

// BoardRoleSummary and BoardTriggerSummary carry the same board config to the
// web page and the TUI's read-only config view.
type BoardRoleSummary struct {
	Name       string `json:"name"`
	Agent      string `json:"agent"`
	Tier       string `json:"tier,omitempty"`
	Model      string `json:"model,omitempty"`
	Effort     string `json:"effort,omitempty"`
	FreshAfter *int   `json:"freshAfter,omitempty"`
}
type BoardTriggerSummary struct {
	On     string `json:"on"`
	When   string `json:"when,omitempty"`
	Agent  string `json:"agent"`
	Prompt string `json:"prompt,omitempty"`
}
type BoardSummary struct {
	DefaultWorkflow string                `json:"defaultWorkflow,omitempty"`
	Roles           []BoardRoleSummary    `json:"roles"`
	Triggers        []BoardTriggerSummary `json:"triggers"`
	MaxFiresPerHour int                   `json:"maxFiresPerHour"`
}

// BoardConfigError is a validation failure tied to a source line.
type BoardConfigError struct {
	Line int
	Err  error
}

func (e *BoardConfigError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %v", e.Line, e.Err)
	}
	return e.Err.Error()
}
func (e *BoardConfigError) Unwrap() error { return e.Err }

var configLineRE = regexp.MustCompile(`line (\d+)`)

// BoardConfigErrorLine returns the 1-based line of a board config error, or 0.
// YAML decoder errors already name their line; semantic errors carry one from
// the node tree.
func BoardConfigErrorLine(err error) int {
	var e *BoardConfigError
	if errors.As(err, &e) {
		return e.Line
	}
	if err == nil {
		return 0
	}
	if m := configLineRE.FindStringSubmatch(err.Error()); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func configErr(line int, format string, args ...any) error {
	return &BoardConfigError{Line: line, Err: fmt.Errorf(format, args...)}
}

// boardYAMLLines remembers where each documented key sits, so a semantic error
// points at its line like a YAML syntax error does.
type boardYAMLLines struct {
	top      map[string]int
	statuses map[string]int
	columns  map[string]int
	roles    map[string]int
	triggers []int
}

func boardConfigLines(b []byte) boardYAMLLines {
	out := boardYAMLLines{top: map[string]int{}, statuses: map[string]int{}, columns: map[string]int{}, roles: map[string]int{}}
	var doc yaml.Node
	if yaml.Unmarshal(b, &doc) != nil || len(doc.Content) == 0 {
		return out
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		out.top[key.Value] = key.Line
		switch key.Value {
		case "statuses":
			for _, item := range sequence(value) {
				if id := mappingValue(item, "id"); id != nil {
					out.statuses[id.Value] = item.Line
				}
			}
		case "columns":
			for _, item := range sequence(value) {
				out.columns[item.Value] = item.Line
			}
		case "roles":
			if value.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(value.Content); j += 2 {
					out.roles[value.Content[j].Value] = value.Content[j].Line
				}
			}
		case "triggers":
			for _, item := range sequence(value) {
				out.triggers = append(out.triggers, item.Line)
			}
		}
	}
	return out
}

func sequence(n *yaml.Node) []*yaml.Node {
	if n == nil || n.Kind != yaml.SequenceNode {
		return nil
	}
	return n.Content
}

func mappingValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func (f Filter) String() string {
	var parts []string
	if f.To != "" {
		parts = append(parts, "to="+f.To)
	}
	if f.From != "" {
		parts = append(parts, "from="+f.From)
	}
	if f.Actor != "" {
		parts = append(parts, "actor="+f.Actor)
	}
	return strings.Join(parts, " ")
}

// Summary is the readable board config shared by `#/board/config` and the TUI.
func (c BoardConfig) Summary() BoardSummary {
	sum := BoardSummary{DefaultWorkflow: c.DefaultWorkflow, MaxFiresPerHour: c.MaxFiresPerHour,
		Roles: []BoardRoleSummary{}, Triggers: []BoardTriggerSummary{}}
	for name, r := range c.Roles {
		sum.Roles = append(sum.Roles, BoardRoleSummary{Name: name, Agent: r.Agent, Tier: r.Tier, Model: r.Model, Effort: r.Effort, FreshAfter: r.FreshAfter})
	}
	slices.SortFunc(sum.Roles, func(a, b BoardRoleSummary) int { return strings.Compare(a.Name, b.Name) })
	for _, t := range c.Triggers {
		sum.Triggers = append(sum.Triggers, BoardTriggerSummary{On: t.On, When: t.When.String(), Agent: t.Agent, Prompt: t.Prompt})
	}
	return sum
}

// DefaultColumns are the statuses a ticket without a workflow can be in.
func DefaultColumns() []string { return []string{"todo", "blocked", "done"} }

// MainRoot deliberately uses the common git dir, so linked worktrees share config.
// mainRoots caches git answers: a directory's main worktree never changes, and the web, scraper and
// sidebar ask for it per ticket and per pane on every refresh (one git exec each, ~5ms).
var mainRoots sync.Map

// The non-repo answer is cached too: a pane at ~ is asked per ticket, and an uncached miss forks git every time.
// ponytail: a dir that becomes a repo after first use keeps its non-repo answer until restart; add a TTL if that bites.
func MainRoot(dir string) string {
	if root, ok := mainRoots.Load(dir); ok {
		return root.(string)
	}
	root := dir
	if abs, err := filepath.Abs(dir); err == nil {
		root = abs
	}
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output(); err == nil {
		root = filepath.Dir(strings.TrimSpace(string(out)))
	}
	mainRoots.Store(dir, root)
	return root
}
func BoardConfigPath(dir string) string              { return filepath.Join(MainRoot(dir), ".tasks", "board.yaml") }
func ParseBoardConfig(b []byte) (BoardConfig, error) { return ParseBoardConfigFor("", b) }

// ParseBoardConfigFor also resolves default_workflow when dir is known.
func ParseBoardConfigFor(dir string, b []byte) (BoardConfig, error) {
	lines := boardConfigLines(b)
	c := BoardConfig{MaxFiresPerHour: 20, defaultWorkflowLine: lines.top["default_workflow"]}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(&c); err != nil && err != io.EOF {
		return c, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, fmt.Errorf("expected one YAML document")
	}
	if c.Statuses != nil && c.Columns != nil {
		return c, configErr(lines.top["columns"], "use statuses or columns, not both")
	}
	if len(c.Statuses) > 0 || len(c.Columns) > 0 {
		c.Legacy = true
		if len(c.Statuses) == 0 {
			for _, id := range c.Columns {
				c.Statuses = append(c.Statuses, Status{ID: id})
			}
		}
		c.Columns = nil
		for _, s := range c.Statuses {
			c.Columns = append(c.Columns, s.ID)
		}
		if c.DefaultWorkflow == "" {
			if hasLegacyJobs(c.Statuses) {
				c.DefaultWorkflow = "board"
			}
		}
		c.Warnings = append(c.Warnings, "statuses are deprecated; define flows in .tasks/workflows/ and set default_workflow")
	} else {
		c.Columns = DefaultColumns()
	}
	if err := c.validateLanes(); err != nil {
		return c, configErr(lines.top["lanes"], "%w", err)
	}
	c.addDeclared()
	for name, r := range c.Roles {
		if err := ValidateEffort(r.Agent, r.Effort); err != nil {
			return c, configErr(lines.roles[name], "role %s: %w", name, err)
		}
		if r.Tier != "" {
			// A tier role's harness is chosen per dispatch, so it cannot pin a model or harness context.
			if r.Agent != "" || r.Model != "" || r.Set() || strings.TrimSpace(name) == "" {
				return c, configErr(lines.roles[name], "role %s: a tier role sets only tier and effort", name)
			}
			continue
		}
		if strings.TrimSpace(name) == "" || strings.TrimSpace(r.Agent) == "" || (r.FreshAfter != nil && *r.FreshAfter < 0) {
			return c, configErr(lines.roles[name], "role requires name and agent")
		}
		if _, err := r.Args(r.Agent, "x"); err != nil {
			return c, configErr(lines.roles[name], "role %s: %w", name, err)
		}
	}
	if c.Legacy {
		if err := validateLegacyStatuses(&c, lines); err != nil {
			return c, err
		}
	}
	if c.MaxFiresPerHour < 1 {
		return c, configErr(lines.top["max_fires_per_hour"], "max_fires_per_hour must be positive")
	}
	for i, t := range c.Triggers {
		line := 0
		if i < len(lines.triggers) {
			line = lines.triggers[i]
		}
		if !slices.Contains([]string{"create", "assign", "status", "comment", "mention", "due"}, t.On) {
			return c, configErr(line, "invalid trigger kind %q", t.On)
		}
		if strings.TrimSpace(t.Agent) == "" {
			return c, configErr(line, "trigger requires agent; replace workflow targets with statuses")
		}
		for _, s := range []string{t.Agent, t.Prompt} {
			if err := ValidateTemplate(s); err != nil {
				return c, configErr(line, "%w", err)
			}
		}
	}
	if err := c.validateDefaultWorkflow(dir); err != nil {
		return c, err
	}
	return c, nil
}

// validateLegacyStatuses keeps old board.yaml files loading. Their statuses
// become the implicit "board" workflow; anything a linear flow cannot express
// is reported as a warning instead of breaking a running board.
func validateLegacyStatuses(c *BoardConfig, lines boardYAMLLines) error {
	seen := map[string]bool{}
	index := map[string]int{}
	for i, s := range c.Statuses {
		if strings.TrimSpace(s.ID) == "" || seen[s.ID] {
			return configErr(lines.statuses[s.ID], "invalid or duplicate status %q", s.ID)
		}
		seen[s.ID], index[s.ID] = true, i
	}
	if !seen["todo"] || !seen["done"] {
		return configErr(lines.top["columns"], "columns must include todo and done")
	}
	adjacent := func(from string, to string) bool {
		if to == "todo" || to == "blocked" || to == "done" {
			return true
		}
		a, ok := index[from]
		b, ok2 := index[to]
		return ok && ok2 && (b == a+1 || b == a-1)
	}
	if len(lines.columns) > 0 {
		c.Warnings = append(c.Warnings, "columns is deprecated; board lanes are fixed (todo, in progress, blocked, done)")
	}
	for _, s := range c.Statuses {
		line := lines.statuses[s.ID]
		if s.MaxLoops < 0 || (s.FreshAfter != nil && *s.FreshAfter < 0) || (s.Role != "" && s.Run != "") {
			return configErr(line, "invalid status %s", s.ID)
		}
		if (s.Role != "" || s.MaxLoops > 0) && !seen["blocked"] {
			return configErr(line, "status %s requires blocked status", s.ID)
		}
		if s.Run != "" && (s.Pass == "" || s.Fail == "") {
			return configErr(line, "run status %s requires pass and fail", s.ID)
		}
		if s.Run == "" && (s.Pass != "" || s.Fail != "") {
			return configErr(line, "pass/fail require run")
		}
		nextSeen := map[string]bool{}
		for _, id := range s.Next {
			if !seen[id] || nextSeen[id] {
				return configErr(line, "invalid next %q", id)
			}
			nextSeen[id] = true
			if !adjacent(s.ID, id) {
				c.Warnings = append(c.Warnings, fmt.Sprintf("status %s: next %q cannot be represented in a linear workflow", s.ID, id))
			}
		}
		for _, id := range []string{s.Pass, s.Fail} {
			if id != "" && !seen[id] {
				return configErr(line, "unknown status %q", id)
			}
			if id != "" && !adjacent(s.ID, id) {
				c.Warnings = append(c.Warnings, fmt.Sprintf("status %s: %q cannot be represented in a linear workflow", s.ID, id))
			}
		}
		if s.Column != "" {
			c.Warnings = append(c.Warnings, fmt.Sprintf("status %s: column is deprecated; lanes are fixed", s.ID))
		}
	}
	return nil
}

func hasLegacyJobs(statuses []Status) bool {
	for _, s := range statuses {
		if s.Role != "" || s.Run != "" {
			return true
		}
	}
	return false
}

func (c BoardConfig) validateDefaultWorkflow(dir string) error {
	if c.DefaultWorkflow == "" || dir == "" {
		return nil
	}
	w, err := FindWorkflow(dir, c, c.DefaultWorkflow)
	if err != nil {
		return configErr(c.defaultWorkflowLine, "default_workflow: %v", err)
	}
	if len(w.Jobs) == 0 {
		return configErr(c.defaultWorkflowLine, "default_workflow %q has no jobs", c.DefaultWorkflow)
	}
	return nil
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
		b, err = os.ReadFile(filepath.Join(apppath.ConfigDir(), "board.yaml"))
	}
	if os.IsNotExist(err) {
		return ParseBoardConfigFor(dir, nil)
	}
	if err != nil {
		return BoardConfig{}, err
	}
	return ParseBoardConfigFor(dir, b)
}
func SaveBoardConfig(dir string, b []byte) error {
	if _, err := ParseBoardConfigFor(dir, b); err != nil {
		return err
	}
	p := BoardConfigPath(dir)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	return AtomicWrite(p, b)
}

// SetYAMLField sets a scalar key in a mapping node; an empty value removes it.
// Shared with the agent editor so both writes keep comments and key order.
func SetYAMLField(m *yaml.Node, key, value string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			continue
		}
		if value == "" {
			// Keep the removed key's comments: move them onto the next key, or
			// the previous key's foot when it was last.
			key, val := m.Content[i], m.Content[i+1]
			head, line, foot := key.HeadComment, key.LineComment, key.FootComment
			if head == "" {
				head = val.HeadComment
			}
			if line == "" {
				line = val.LineComment
			}
			if foot == "" {
				foot = val.FootComment
			}
			m.Content = slices.Delete(m.Content, i, i+2)
			if i < len(m.Content) {
				next := m.Content[i]
				if head != "" && next.HeadComment == "" {
					next.HeadComment = head
				}
				if line != "" && next.LineComment == "" {
					next.LineComment = line
				}
				if foot != "" {
					if next.HeadComment == "" {
						next.HeadComment = foot
					} else {
						next.HeadComment += "\n" + foot
					}
				}
			} else if len(m.Content) > 0 {
				last := m.Content[len(m.Content)-1]
				for _, c := range []string{head, line, foot} {
					if c == "" {
						continue
					}
					if last.FootComment == "" {
						last.FootComment = c
					} else {
						last.FootComment += "\n" + c
					}
				}
			}
		} else {
			m.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Value: value}
		}
		return
	}
	if value != "" {
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, &yaml.Node{Kind: yaml.ScalarNode, Value: value})
	}
}

// YAMLValue returns the value node for key, or nil.
func YAMLValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// SetDefaultWorkflow changes only default_workflow in .tasks/board.yaml. It
// edits the YAML node tree so comments and key order survive, validates the
// result (including that the workflow exists), then replaces the file. An empty
// name removes the key; a missing file is created from BoardExample.
func SetDefaultWorkflow(dir, name string) error {
	path := BoardConfigPath(dir)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if name == "" {
			return nil
		}
		b, err = []byte(BoardExample), nil
	}
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	SetYAMLField(doc.Content[0], "default_workflow", name)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if _, err := ParseBoardConfigFor(dir, buf.Bytes()); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return AtomicWrite(path, buf.Bytes())
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

// Lane is the board column a status is shown in. Lanes are fixed; every
// workflow job status lands in "in progress" and the card shows the real status.
func (c BoardConfig) Lane(status string) string {
	for lane, ids := range c.LaneStatuses {
		if slices.Contains(ids, status) {
			return lane
		}
	}
	switch status {
	case "todo", "blocked", "done":
		return status
	}
	return "in progress"
}

// Lanes lists the board columns in display order.
func (c BoardConfig) Lanes() []string { return []string{"todo", "in progress", "blocked", "done"} }

// validateLanes checks the declared lanes: known lane names, plain status ids,
// each status in one lane only.
func (c BoardConfig) validateLanes() error {
	seen := map[string]string{}
	for lane, ids := range c.LaneStatuses {
		if !slices.Contains(c.Lanes(), lane) {
			return fmt.Errorf("lanes: unknown lane %q (use %s)", lane, strings.Join(c.Lanes(), ", "))
		}
		for _, id := range ids {
			if id == "" || strings.ContainsAny(id, " \t\n,/\\") {
				return fmt.Errorf("lanes: invalid status %q", id)
			}
			if other, dup := seen[id]; dup {
				return fmt.Errorf("lanes: status %q is in both %q and %q", id, other, lane)
			}
			seen[id] = lane
		}
	}
	return nil
}

// addDeclared makes the declared statuses valid for the ticket and orders the
// columns by lane (stable inside a lane), so H/L and the picker follow the board.
func (c *BoardConfig) addDeclared() {
	for _, lane := range c.Lanes() {
		for _, id := range c.LaneStatuses[lane] {
			if !slices.Contains(c.Columns, id) {
				c.Columns = append(c.Columns, id)
				if len(c.Statuses) > 0 {
					c.Statuses = append(c.Statuses, Status{ID: id})
				}
			}
		}
	}
	rank := func(id string) int { // lane first, then the order the lane declares
		at := slices.Index(c.LaneStatuses[c.Lane(id)], id)
		if at < 0 {
			at = 999
		}
		return slices.Index(c.Lanes(), c.Lane(id))*1000 + at
	}
	slices.SortStableFunc(c.Columns, func(a, b string) int { return rank(a) - rank(b) })
}
