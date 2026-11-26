// Package schedule fires scheduled workflow tickets and agent prompts from the
// scraper loop. Entries live in <repo>/.tasks/schedules.yml (registered repos
// only) and ~/.config/tagent/schedules.yml; each file keeps its machine state
// (last run, last ticket) beside it in schedules.state.json.
package schedule

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
	"tagent/internal/apppath"
	"tagent/internal/board"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

// Dispatch delivers a send action. The CLI sets it: it owns dispatchBoardAction.
var Dispatch board.Fire

// Entry is one schedule. Err is set for an invalid entry, which never fires.
type Entry struct {
	Name, At, Workflow, Title string
	Every                     time.Duration
	Enabled, Global           bool
	Inputs                    map[string]string
	Dir                       string // workflow and send run here: the repo root locally
	Agent, Prompt             string
	File                      string
	Line                      int
	Err                       error
}

// Rule is the schedule as written: "at 08:00" or "every 30m".
func (e Entry) Rule() string {
	if e.At != "" {
		return "at " + e.At
	}
	return "every " + e.Every.String()
}

// Next is the first slot after last: last+every, or the next HH:MM after last.
func (e Entry) Next(last time.Time) time.Time {
	if e.At == "" {
		return last.Add(e.Every)
	}
	t, _ := time.Parse("15:04", e.At)
	n := time.Date(last.Year(), last.Month(), last.Day(), t.Hour(), t.Minute(), 0, 0, last.Location())
	if !n.After(last) {
		n = n.AddDate(0, 0, 1)
	}
	return n
}

var known = map[string]bool{"name": true, "at": true, "every": true, "enabled": true, "workflow": true, "title": true, "inputs": true, "dir": true, "send": true}

type raw struct {
	Name     string            `yaml:"name"`
	At       string            `yaml:"at"`
	Every    string            `yaml:"every"`
	Enabled  *bool             `yaml:"enabled"`
	Workflow string            `yaml:"workflow"`
	Title    string            `yaml:"title"`
	Inputs   map[string]string `yaml:"inputs"`
	Dir      string            `yaml:"dir"`
	Send     *struct {
		Agent  string `yaml:"agent"`
		Prompt string `yaml:"prompt"`
	} `yaml:"send"`
}

// GlobalFile is ~/.config/tagent/schedules.yml.
func GlobalFile() string { return filepath.Join(apppath.ConfigDir(), "schedules.yml") }

// LocalFile is the schedules file of the repo at root.
func LocalFile(root string) string { return filepath.Join(root, ".tasks", "schedules.yml") }

// TrustFile lists the repos whose local schedules may run: one absolute path per
// line. It sits outside the repo, so a clone cannot trust itself.
func TrustFile() string { return filepath.Join(apppath.ConfigDir(), "schedule-trust") }

// Trusted is the trust list. A missing or unreadable list trusts nothing.
func Trusted() []string {
	b, err := os.ReadFile(TrustFile())
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// Trust adds or removes root in the trust list.
func Trust(root string, on bool) error {
	var keep []string
	for _, r := range Trusted() {
		if r != root {
			keep = append(keep, r)
		}
	}
	if on {
		keep = append(keep, root)
	}
	if err := os.MkdirAll(filepath.Dir(TrustFile()), 0700); err != nil {
		return err
	}
	return tasks.AtomicWrite(TrustFile(), []byte(strings.Join(keep, "\n")+"\n"))
}

// ErrSkip: a due slot was passed over on purpose; the state notes it, not an error.
var ErrSkip = errors.New("skipped")

// Load reads a schedules file. root is the repo for a local file, "" for the global one.
// A missing file has no entries; a file that is not a YAML list is an error.
func Load(file, root string) ([]Entry, error) {
	b, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := Parse(b, file, root == "")
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if !entries[i].Global {
			entries[i].Dir = root
		}
	}
	return entries, nil
}

// Parse validates the entries of one file. An invalid entry comes back with Err
// set and disabled; its siblings are unaffected.
func Parse(b []byte, file string, global bool) ([]Entry, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	list := doc.Content[0]
	if list.Kind != yaml.SequenceNode {
		return nil, &workflow.Error{Line: list.Line, Msg: fmt.Sprintf("%s:%d: schedules must be a list of entries", file, list.Line)}
	}
	var out []Entry
	seen := map[string]bool{}
	for _, item := range list.Content {
		e := Entry{File: file, Global: global, Enabled: true, Line: item.Line}
		if msg := parseItem(item, &e, seen); msg != "" {
			e.Enabled = false
			e.Err = &workflow.Error{Line: item.Line, Msg: fmt.Sprintf("%s:%d: %s", file, item.Line, msg)}
		}
		out = append(out, e)
	}
	return out, nil
}

// parseItem fills e from one list item and returns the first problem, or "".
func parseItem(n *yaml.Node, e *Entry, seen map[string]bool) string {
	if n.Kind != yaml.MappingNode {
		return "entry must be a mapping"
	}
	for i := 0; i < len(n.Content); i += 2 {
		if k := n.Content[i].Value; !known[k] {
			return "unknown key " + k
		}
	}
	var r raw
	if err := n.Decode(&r); err != nil {
		return err.Error()
	}
	switch {
	case r.Name == "":
		return "name is required"
	case seen[r.Name]:
		return "duplicate name " + r.Name
	}
	seen[r.Name] = true
	e.Name = r.Name
	if (r.At == "") == (r.Every == "") {
		return "exactly one of at or every"
	}
	if r.At != "" {
		if _, err := time.Parse("15:04", r.At); err != nil {
			return "at must be HH:MM, got " + strconv.Quote(r.At)
		}
		e.At = r.At
	} else {
		d, err := time.ParseDuration(r.Every)
		if err != nil || d < time.Minute {
			return "every must be a duration of at least 1m, got " + strconv.Quote(r.Every)
		}
		e.Every = d
	}
	if r.Enabled != nil {
		e.Enabled = *r.Enabled
	}
	if (r.Workflow == "") == (r.Send == nil) {
		return "exactly one of workflow or send"
	}
	if r.Dir != "" && !e.Global {
		return "dir is not allowed in a repo's schedules.yml: it runs in the repo"
	}
	home, _ := os.UserHomeDir()
	e.Dir = expandHome(r.Dir, home)
	if r.Send != nil {
		if r.Title != "" || len(r.Inputs) > 0 {
			return "title and inputs go with workflow, not send"
		}
		if r.Send.Agent == "" || r.Send.Prompt == "" {
			return "send needs agent and prompt"
		}
		e.Agent, e.Prompt = r.Send.Agent, r.Send.Prompt
		if e.Dir == "" && e.Global {
			e.Dir = home
		}
		return ""
	}
	if e.Global && e.Dir == "" {
		return "workflow in the global schedules needs dir"
	}
	e.Workflow, e.Title, e.Inputs = r.Workflow, r.Title, r.Inputs
	return ""
}

func expandHome(p, home string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[1:])
	}
	return p
}

// State is what a schedule did last: its slot, the ticket it made, and a note.
type State struct {
	Last   time.Time `json:"last"`
	Ticket string    `json:"ticket,omitempty"`
	Note   string    `json:"note,omitempty"`
}

type states map[string]State

func statePath(file string) string { return filepath.Join(filepath.Dir(file), "schedules.state.json") }

// update runs fn on the state under an flock and writes it back when fn reports
// a change. An unreadable state is an error, so nothing fires without it.
func update(file string, fn func(states) bool) error {
	sp := statePath(file)
	if err := os.MkdirAll(filepath.Dir(sp), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(sp+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	s := states{}
	b, err := os.ReadFile(sp)
	if err == nil {
		if err = json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("%s: %w", sp, err)
		}
		if s == nil {
			s = states{}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if !fn(s) {
		return nil
	}
	b, err = json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return tasks.AtomicWrite(sp, b)
}

// StateOf is the recorded state of e (zero when it never ran).
func StateOf(e Entry) (st State) {
	update(e.File, func(s states) bool { st = s[e.Name]; return false })
	return st
}

// openTicket: a recorded ticket that still exists and is not done or closed.
func openTicket(dir, id string) bool {
	t, err := tasks.Get(tasks.Scope(dir), id)
	return err == nil && t.Status != "done" && t.Status != "closed"
}

// Fire runs one entry and returns the ticket it created, if any.
type Fire func(Entry) (string, error)

// Fires is the production Fire: a workflow creates a ticket, a send dispatches a prompt.
func Fires(e Entry) (string, error) {
	if e.Workflow != "" {
		// A global schedule must not make a repo's own schedules.yml run: it needs a
		// registered repo, and only the CLI registers one (local schedules need trust).
		if root := tasks.MainRoot(e.Dir); e.Global && !slices.Contains(board.Registered(), root) {
			return "", fmt.Errorf("%s is not registered with tagent: run `tagent workflow run` there once", root)
		}
		return board.RunWorkflow(e.Dir, e.Workflow, e.Title, e.Inputs, "schedule")
	}
	if Dispatch == nil {
		return "", errors.New("no dispatcher for send")
	}
	a, err := board.AgentAction(e.Dir, e.Agent, e.Prompt)
	if err != nil {
		return "", err
	}
	return "", Dispatch(a)
}

// Check fires the due entries of one file at now. An entry with no state is
// armed (last = now) and does not fire. A due slot is claimed (last = now) under
// the lock before it runs, so a restart cannot fire it twice; a run that was
// missed fires once, not once per missed slot.
func Check(file, root string, now time.Time, fire Fire) error {
	entries, err := Load(file, root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Err != nil || !e.Enabled {
			continue
		}
		due := false
		err = update(file, func(s states) bool {
			st, seen := s[e.Name]
			if !seen {
				s[e.Name] = State{Last: now}
				return true
			}
			if e.Next(st.Last).After(now) {
				return false
			}
			st.Last = now
			if e.Workflow != "" && st.Ticket != "" && openTicket(e.Dir, st.Ticket) {
				st.Note = "skipped: #" + st.Ticket + " still open"
				s[e.Name] = st
				return true
			}
			st.Note = ""
			s[e.Name] = st
			due = true
			return true
		})
		if err != nil {
			return err
		}
		if !due {
			continue
		}
		ticket, ferr := fire(e)
		if err = record(e, ticket, ferr); err != nil {
			return err
		}
	}
	return nil
}

func record(e Entry, ticket string, ferr error) error {
	return update(e.File, func(s states) bool {
		st := s[e.Name]
		if ticket != "" {
			st.Ticket = ticket
		}
		st.Note = ""
		if errors.Is(ferr, ErrSkip) {
			st.Note = ferr.Error()
		} else if ferr != nil {
			st.Note = "error: " + ferr.Error()
		}
		s[e.Name] = st
		return true
	})
}

// RunNow fires one entry now, ignoring enabled and the open-ticket guard.
func RunNow(e Entry, now time.Time, fire Fire) (string, error) {
	if e.Err != nil {
		return "", e.Err
	}
	if err := update(e.File, func(s states) bool {
		st := s[e.Name]
		st.Last = now
		s[e.Name] = st
		return true
	}); err != nil {
		return "", err
	}
	ticket, ferr := fire(e)
	if err := record(e, ticket, ferr); err != nil {
		return ticket, err
	}
	return ticket, ferr
}

// Toggle sets the enabled key of entry e in its file, keeping the rest of the
// document as yaml.v3 keeps it. Resuming also arms the schedule (last = now), so
// it does not fire at once for a slot it missed while paused.
func Toggle(e Entry, on bool, now time.Time) error {
	file, err := filepath.EvalSymlinks(e.File)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err = yaml.Unmarshal(b, &doc); err != nil {
		return err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.SequenceNode {
		return fmt.Errorf("%s: not a list of schedules", file)
	}
	found := false
	for _, item := range doc.Content[0].Content {
		if item.Kind == yaml.MappingNode && nameOf(item) == e.Name {
			setKey(item, "enabled", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(on)})
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no schedule %s in %s", e.Name, file)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err = enc.Encode(&doc); err != nil {
		return err
	}
	if err = enc.Close(); err != nil {
		return err
	}
	if err = tasks.AtomicWrite(file, buf.Bytes()); err != nil {
		return err
	}
	if on {
		return update(e.File, func(s states) bool {
			st := s[e.Name]
			st.Last = now
			s[e.Name] = st
			return true
		})
	}
	return nil
}

func nameOf(item *yaml.Node) string {
	for i := 0; i < len(item.Content); i += 2 {
		if item.Content[i].Value == "name" {
			return item.Content[i+1].Value
		}
	}
	return ""
}

func setKey(item *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i < len(item.Content); i += 2 {
		if item.Content[i].Value == key {
			item.Content[i+1] = val
			return
		}
	}
	item.Content = append(item.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, val)
}
