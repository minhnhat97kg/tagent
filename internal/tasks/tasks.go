// Package tasks stores a shared task board for all worktrees of a repository.
package tasks

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"tagent/internal/sharedctx"
)

type Task struct {
	Workflow    string                       `json:"workflow,omitempty"`
	Inputs      map[string]string            `json:"inputs,omitempty"`
	Outputs     map[string]map[string]string `json:"outputs,omitempty"`
	Needs       []string                     `json:"needs,omitempty"`
	Title       string                       `json:"title"`
	Due         string                       `json:"due,omitempty"`
	Comments    []TaskComment                `json:"comments"`
	ID          string                       `json:"id"`
	Text        string                       `json:"text"`
	Status      string                       `json:"status"`
	Assignees   []string                     `json:"assignees"`
	Created     time.Time                    `json:"created"`
	Updated     time.Time                    `json:"updated"`
	CompletedAt time.Time                    `json:"completed_at,omitempty"`
	Note        string                       `json:"note,omitempty"` // what was done, written when the task is finished
}

var scopeRoots sync.Map // scope -> main worktree root; populated by Scope

type MutationOptions struct {
	Attachments []string
	Workflow    string
	Inputs      map[string]string
	Outputs     map[string]string
	Actor       string
	Needs       []string
}

var scopeCache sync.Map // dir → scope: it runs git, and the UI asks for it many times a frame

func Scope(dir string) string {
	if v, ok := scopeCache.Load(dir); ok {
		return v.(string)
	}
	s := computeScope(dir)
	scopeRoots.Store(s, MainRoot(dir))
	scopeCache.Store(dir, s)
	return s
}

func computeScope(dir string) string {
	root := dir
	label := filepath.Base(dir)
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output(); err == nil {
		root = strings.TrimSpace(string(out))
		label = filepath.Base(filepath.Dir(root))
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	sum := sha1.Sum([]byte(root))
	return label + "-" + hex.EncodeToString(sum[:4])
}
func boardDir(scope string) string { return filepath.Join(sharedctx.Dir(scope), "tasks") }
func file(scope, id string) string { return filepath.Join(boardDir(scope), id+".json") }
func validID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}
func ValidateAssignees(names []string) error {
	for _, name := range names {
		if name == "" || strings.ContainsAny(name, " \t\n,/\\") {
			return fmt.Errorf("invalid assignee %q", name)
		}
	}
	return nil
}
func List(scope string) []Task {
	files, _ := filepath.Glob(filepath.Join(boardDir(scope), "*.json"))
	out := make([]Task, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var t Task
		if json.Unmarshal(b, &t) == nil && validID(t.ID) {
			normalize(&t)
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out
}

// nextID numbers tickets 1, 2, 3… per repository so people can say "ticket 12".
// Older tickets keep their long time-based ids.
func nextID(scope string) string {
	// Deleted tickets keep their events, so their numbers are never reused.
	n := 0
	seen := func(id string) {
		if v, err := strconv.Atoi(id); err == nil && len(id) <= 6 && v > n {
			n = v
		}
	}
	for _, t := range List(scope) {
		seen(t.ID)
	}
	events, _ := ReadEvents(scope, 0)
	for _, e := range events {
		seen(e.Task)
	}
	return strconv.Itoa(n + 1)
}
func Get(scope, id string) (Task, error) {
	if !validID(id) {
		return Task{}, errors.New("invalid task id")
	}
	b, err := os.ReadFile(file(scope, id))
	if err != nil {
		return Task{}, err
	}
	var t Task
	err = json.Unmarshal(b, &t)
	normalize(&t)
	return t, err
}
func withLock(scope string, fn func() error) error {
	if err := os.MkdirAll(boardDir(scope), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(boardDir(scope), ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
func write(scope string, t Task) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(boardDir(scope), ".task-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), file(scope, t.ID))
}
func Add(scope, text string, assignees []string, opts ...MutationOptions) (Task, error) {

	text = strings.TrimSpace(text)
	if text == "" {
		return Task{}, errors.New("empty task")
	}
	for i := range assignees {
		assignees[i] = strings.TrimSpace(assignees[i])
	}
	if err := ValidateAssignees(assignees); err != nil {
		return Task{}, err
	}
	now := time.Now()
	t := Task{Title: strings.SplitN(text, "\n", 2)[0], Comments: []TaskComment{}, Text: text, Status: "todo", Assignees: assignees, Created: now, Updated: now}
	err := withLock(scope, func() error {
		t.ID = nextID(scope)
		if len(opts) > 0 {
			t.Needs = opts[0].Needs
			t.Workflow = opts[0].Workflow
			if t.Workflow != "" {
				dir, ok := scopeRoots.Load(scope)
				if !ok {
					return fmt.Errorf("unknown repository scope")
				}
				base, e := configForScope(scope)
				if e != nil {
					return e
				}
				w, e := FindWorkflow(dir.(string), base, t.Workflow)
				if e != nil {
					return e
				}
				t.Inputs, e = w.Values(opts[0].Inputs)
				if e != nil {
					return e
				}
			}
			seen := map[string]bool{}
			for _, id := range t.Needs {
				if seen[id] {
					return fmt.Errorf("duplicate dependency %s", id)
				}
				seen[id] = true
			}
			for _, id := range t.Needs {
				if id == "" {
					continue
				}
				if _, err := Get(scope, id); err != nil {
					return fmt.Errorf("task reference %s: %w", id, err)
				}
			}
		}
		if err := write(scope, t); err != nil {
			return err
		}
		events := []Event{{Kind: "create"}}
		events = append(events, assignmentEvents(nil, assignees)...)
		return appendEvents(scope, t.ID, actor(opts), events)
	})
	return t, err
}

// SetWorkflow attaches a recipe without changing a ticket that cannot accept it.
func SetWorkflow(scope, id, name string, given map[string]string) error {
	return withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		if t.Status != "todo" {
			return fmt.Errorf("workflow can only be set in todo")
		}
		root, ok := scopeRoots.Load(scope)
		if !ok {
			return fmt.Errorf("unknown repository scope")
		}
		base, err := configForScope(scope)
		if err != nil {
			return err
		}
		w, err := FindWorkflow(root.(string), base, name)
		if err != nil {
			return err
		}
		values := map[string]string{}
		for k, v := range given {
			values[k] = v
		}
		var missing []string
		for k, v := range w.Inputs {
			if v.Required && v.Default == "" && values[k] == "" {
				missing = append(missing, k)
			}
		}
		sort.Strings(missing)
		if len(missing) > 1 {
			return fmt.Errorf("missing inputs: %s; use workflow run %s --input name=value", strings.Join(missing, ", "), name)
		}
		if len(missing) == 1 {
			values[missing[0]] = t.Text
		}
		t.Inputs, err = w.Values(values)
		if err != nil {
			return err
		}
		t.Workflow, t.Updated = name, time.Now()
		return write(scope, t)
	})
}

// AttachWorkflow assigns a workflow to a ticket that is already out of todo.
// The migrate command uses it; normal starts go through SetWorkflow.
func AttachWorkflow(scope, id, name string) error {
	return withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		// "board" is the implicit workflow of a deprecated board.yaml; migrate
		// replaces it with the configured default_workflow.
		if t.Workflow != "" && t.Workflow != "board" {
			return fmt.Errorf("ticket already has workflow %q", t.Workflow)
		}
		root, ok := scopeRoots.Load(scope)
		if !ok {
			return fmt.Errorf("unknown repository scope")
		}
		base, err := configForScope(scope)
		if err != nil {
			return err
		}
		w, err := FindWorkflow(root.(string), base, name)
		if err != nil {
			return err
		}
		values := map[string]string{}
		var missing []string
		for k, v := range w.Inputs {
			if v.Required && v.Default == "" && values[k] == "" {
				missing = append(missing, k)
			}
		}
		sort.Strings(missing)
		if len(missing) > 1 {
			return fmt.Errorf("missing inputs: %s", strings.Join(missing, ", "))
		}
		if len(missing) == 1 {
			values[missing[0]] = t.Text
		}
		t.Inputs, err = w.Values(values)
		if err != nil {
			return err
		}
		t.Workflow, t.Updated = name, time.Now()
		return write(scope, t)
	})
}

func Update(scope, id, status string, assignees []string, opts ...MutationOptions) (Task, error) {
	if assignees != nil {
		for i := range assignees {
			assignees[i] = strings.TrimSpace(assignees[i])
		}
		if err := ValidateAssignees(assignees); err != nil {
			return Task{}, err
		}
	}
	var t Task
	var notice *statusNotice
	err := withLock(scope, func() error {
		var err error
		t, err = Get(scope, id)
		if err != nil {
			return err
		}
		if status != "" {
			cfg, err := configForTaskScope(scope, t)
			if err != nil {
				return err
			}
			cfg, status, _, err = attachDefault(scope, &t, cfg, status)
			if err != nil {
				return err
			}
			status = jobStatus(cfg, status)
			if !slices.Contains(cfg.Columns, status) {
				return fmt.Errorf("invalid status %q", status)
			}
		}
		from := t.Status
		events := assignmentEvents(t.Assignees, assignees)
		if status != "" && status != t.Status {
			events = append(events, Event{Kind: "status", From: t.Status, To: status})
		}
		if len(events) == 0 {
			return nil
		}
		if status != "" {
			if status == "done" && t.Status != "done" {
				t.CompletedAt = time.Now()
			}
			if status != "done" {
				t.CompletedAt = time.Time{}
			}
			t.Status = status
		}
		if assignees != nil {
			t.Assignees = assignees
		}
		if from != t.Status {
			notice = newStatusNotice(scope, t, from, t.Status)
		}
		t.Updated = time.Now()
		if err := write(scope, t); err != nil {
			return err
		}
		return appendEvents(scope, t.ID, actor(opts), events)
	})
	if err == nil {
		notice.send()
	}
	return t, err
}
func Remove(scope, id string) error {
	if !validID(id) {
		return errors.New("invalid task id")
	}
	return withLock(scope, func() error { return os.Remove(file(scope, id)) })
}

func Edit(scope, id, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("empty task")
	}
	return withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		t.Text = text
		t.Updated = time.Now()
		return write(scope, t)
	})
}

// SetNote records what was done on a task (shown in its detail view).
func SetNote(scope, id, note string) error {
	return withLock(scope, func() error {
		t, err := Get(scope, id)
		if err != nil {
			return err
		}
		t.Note = strings.TrimSpace(note)
		t.Updated = time.Now()
		return write(scope, t)
	})
}
