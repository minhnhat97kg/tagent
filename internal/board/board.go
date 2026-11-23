// Package board dispatches persisted task events. Reservations are saved before
// external side effects; an interrupted fire is deliberately never replayed.
package board

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

type Action struct {
	FreshAfter                                        int
	ResumePrompt                                      func(int) string
	CommentCount                                      int
	OnError                                           func(error)
	Role, Model, Effort, Run, Pass, Fail, BlockReason string
	Dir, ExecDir, Agent, Prompt                       string
	Job                                               string
	Event                                             tasks.Event
	Trigger                                           int
	Close                                             bool // ticket reached done: close its role panes
}
type Fire func(Action) error
type state struct {
	LastSeq   int64                  `json:"last_seq"`
	Fires     map[string][]time.Time `json:"fires"`
	LastError string                 `json:"last_error,omitempty"`
}

var ErrLocked = errors.New("board dispatcher already running")

func Run(ctx context.Context, dir string, fire Fire) error {
	dir = tasks.MainRoot(dir)
	scope := tasks.Scope(dir)
	base := tasks.BoardDir(scope)
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(base, "dispatch.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return ErrLocked
		}
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	path := filepath.Join(base, "dispatch.state")
	s := state{Fires: map[string][]time.Time{}}
	if b, e := os.ReadFile(path); e == nil {
		if err = json.Unmarshal(b, &s); err != nil {
			return err
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if s.Fires == nil {
		s.Fires = map[string][]time.Time{}
	}
	save := func() error {
		b, e := json.Marshal(s)
		if e != nil {
			return e
		}
		return tasks.AtomicWrite(path, b)
	}
	type failure struct {
		action Action
		err    error
	}
	failures := make(chan failure, 64)
	audit := func(a Action, outcome string) error {
		target := a.Agent
		if a.Run != "" {
			target = "CI run"
		}
		return tasks.DispatchAudit(scope, a.Event.Task, fmt.Sprintf("%s: %s (%s; seq %d trigger %d)", target, outcome, a.Event.Kind, a.Event.Seq, a.Trigger))
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if err = ctx.Err(); err != nil {
			return nil
		}
		select {
		case e := <-failures:
			s.LastError = e.err.Error()
			if err = audit(e.action, "async failure: "+e.err.Error()); err != nil {
				return err
			}
			if err = save(); err != nil {
				return err
			}
		default:
		}
		cfg, e := tasks.LoadBoardConfig(dir)
		if e != nil {
			s.LastError = e.Error()
			if err = save(); err != nil {
				return err
			}
		} else {
			events, e := tasks.ReadEvents(scope, s.LastSeq)
			if e != nil {
				return e
			}
			for _, ev := range events {
				if ctx.Err() != nil {
					return nil
				}
				// Reserve the whole event before any fire. After a crash its
				// remaining actions are dropped too: no replay or config-index ambiguity.
				s.LastSeq = ev.Seq
				if err = save(); err != nil {
					return err
				}
				task, e := tasks.Get(scope, ev.Task)
				if e != nil {
					s.LastError = e.Error()
					if err = save(); err != nil {
						return err
					}
					continue
				}
				actions, e := Actions(dir, cfg, ev, task)
				if e != nil {
					s.LastError = e.Error()
					if err = save(); err != nil {
						return err
					}
					continue
				}
				for _, a := range actions {
					if ctx.Err() != nil {
						return nil
					}
					if a.Close {
						if err = audit(a, "reserved; outcome uncertain"); err != nil {
							return err
						}
						e = fire(a)
						outcome := "accepted"
						if e != nil {
							s.LastError = e.Error()
							outcome = "failure: " + e.Error()
						}
						if err = audit(a, outcome); err != nil {
							return err
						}
						if err = save(); err != nil {
							return err
						}
						continue
					}
					if a.BlockReason != "" {
						if e = tasks.FinishEntry(scope, task.ID, "blocked", a.BlockReason, ev.Seq, true, tasks.MutationOptions{Actor: "system"}); e != nil {
							s.LastError = e.Error()
						}
						if err = save(); err != nil {
							return err
						}
						continue
					}
					now := time.Now().UTC()
					recent := s.Fires[ev.Task][:0]
					for _, t := range s.Fires[ev.Task] {
						if now.Sub(t) < time.Hour {
							recent = append(recent, t)
						}
					}
					s.Fires[ev.Task] = recent
					allowed := len(recent) < cfg.MaxFiresPerHour
					if allowed {
						s.Fires[ev.Task] = append(recent, now)
					}
					if err = save(); err != nil {
						return err
					}
					if !allowed {
						if err = audit(a, "suppressed: hourly rate limit; wait then task retry "+task.ID); err != nil {
							return err
						}
						continue
					}
					if err = audit(a, "reserved; outcome uncertain"); err != nil {
						return err
					}
					a.OnError = func(e error) {
						select {
						case failures <- failure{a, e}:
						case <-ctx.Done():
						}
					}
					if e = fire(a); e != nil {
						s.LastError = fmt.Sprintf("seq %d trigger %d: %v", ev.Seq, a.Trigger, e)
						if err = audit(a, "failure: "+e.Error()); err != nil {
							return err
						}
					} else {
						target := a.Agent
						if a.Run != "" {
							target = "CI run"
						}
						e = tasks.DispatchAudit(scope, task.ID, fmt.Sprintf("▶ triggered %s: accepted, not completed (%s; seq %d trigger %d)", target, ev.Kind, ev.Seq, a.Trigger))
						if e != nil {
							return e
						}
					}
					if err = save(); err != nil {
						return err
					}
				}
				// Persist consumption even for events with no matching actions.
				if err = save(); err != nil {
					return err
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}
func match(filter, value string) bool { return filter == "" || filter == "*" || filter == value }
func Actions(dir string, cfg tasks.BoardConfig, ev tasks.Event, t tasks.Task) ([]Action, error) {
	var err error
	cfg, err = tasks.ConfigForTask(dir, cfg, t)
	if err != nil {
		return nil, err
	}
	if ev.Actor == "system" {
		return nil, nil
	}
	// A ticket whose dependencies are not done is never dispatched: the CLI, the
	// TUI and the web already refuse to start it, so this only catches events
	// written by an older client or a script.
	if ev.Kind == "status" || ev.Kind == "retry" {
		if len(tasks.OpenNeeds(tasks.Scope(dir), t)) > 0 {
			return nil, nil
		}
	}
	var comment tasks.TaskComment
	for _, c := range t.Comments {
		if c.ID == ev.CommentID {
			comment = c
			break
		}
	}
	names := []string{""}
	if ev.Kind == "mention" {
		names = ev.Names
	}
	var out []Action
	if ev.Kind == "retry" {
		if ev.To != t.Status {
			return nil, nil
		}
		history, err := tasks.ReadEvents(tasks.Scope(dir), ev.Seq)
		if err != nil {
			return nil, err
		}
		for _, e := range history {
			if e.Task == ev.Task && e.Kind == "status" {
				return nil, nil
			}
		}
	}
	if (ev.Kind == "status" && ev.From != ev.To) || ev.Kind == "retry" {
		for _, s := range cfg.Statuses {
			if s.ID != ev.To {
				continue
			}
			a := Action{Dir: dir, Event: ev, Trigger: -1, Agent: s.Role, Role: s.Role, Run: s.Run, Pass: s.Pass, Fail: s.Fail}
			if s.MaxLoops > 0 {
				history, err := tasks.ReadEvents(tasks.Scope(dir), 0)
				if err != nil {
					return nil, err
				}
				count := 0
				for _, e := range history {
					if e.Seq > ev.Seq || e.Task != ev.Task || (e.Kind != "status" && e.Kind != "retry") || (e.Kind == "status" && e.From == e.To) {
						continue
					}
					if e.From == "blocked" {
						count = 0
					}
					if e.To == s.ID {
						count++
					}
				}
				if count > s.MaxLoops {
					a.BlockReason = fmt.Sprintf("Status %s exceeded max_loops %d (entry %d)", s.ID, s.MaxLoops, count)
					return []Action{a}, nil
				}
			}
			a.FreshAfter = 3
			if r, ok := cfg.Roles[s.Role]; ok {
				if r.FreshAfter != nil {
					a.FreshAfter = *r.FreshAfter
				}
				a.Role = r.Agent
				a.Model = r.Model
				a.Effort = r.Effort
			}
			if s.FreshAfter != nil {
				a.FreshAfter = *s.FreshAfter
			}
			a.Job = s.Job
			if s.Model != "" {
				a.Model = s.Model
			}
			a.Run = workflow.Expand(a.Run, t.Inputs, t.Outputs, t.ID, t.Title)
			next := s.Next
			if next == nil {
				next = cfg.Columns
			}
			a.Prompt = statusPrompt(t, s, ev, next, -1)
			a.CommentCount = len(t.Comments)
			a.ResumePrompt = func(since int) string { return statusPrompt(t, s, ev, next, since) }
			if s.Role != "" || s.Run != "" {
				out = append(out, a)
			}
		}
	}
	if ev.Kind == "status" && ev.From != ev.To && cfg.Lane(ev.To) == "done" {
		out = append(out, Action{Dir: dir, Event: ev, Trigger: -1, Close: true})
	}
	if ev.Kind == "retry" {
		return out, nil
	}
	for i, tr := range cfg.Triggers {
		if tr.On != ev.Kind {
			continue
		}
		for _, name := range names {
			to := ev.To
			if ev.Kind == "mention" {
				to = name
			}
			if !match(tr.When.To, to) || !match(tr.When.From, ev.From) || !match(tr.When.Actor, ev.Actor) {
				continue
			}
			values := map[string]string{"task.id": t.ID, "task.title": t.Title, "task.text": t.Text, "task.status": t.Status, "task.due": t.Due, "task.assignees": strings.Join(t.Assignees, ","), "event.kind": ev.Kind, "event.actor": ev.Actor, "event.from": ev.From, "event.to": to, "event.name": name, "comment.body": comment.Body, "comment.author": comment.Author}
			expand := func(s string) string { return expand(s, values) }
			a := Action{Dir: dir, Agent: expand(tr.Agent), Prompt: expand(tr.Prompt), Event: ev, Trigger: i}
			if r, ok := cfg.Roles[a.Agent]; ok {
				a.Role = r.Agent
				a.Model = r.Model
				a.Effort = r.Effort
			}
			target := a.Agent
			if target == "" {
				return nil, fmt.Errorf("empty trigger target")
			}
			if ev.Actor == target {
				continue
			}
			if a.Prompt == "" {
				a.Prompt = fmt.Sprintf("Task %s (%s): %s changed by %s. Read `tagent task show %s` and reply with `tagent task comment %s <text>`.", t.ID, t.Title, ev.Kind, ev.Actor, t.ID, t.ID)
			}
			if ev.Kind == "mention" {
				// A reused pane gets only what it has not seen; a fresh one gets the brief first.
				tail := a.Prompt
				a.Prompt = tasks.Brief(t) + tail + "\n"
				a.CommentCount = len(t.Comments)
				a.ResumePrompt = func(since int) string { return mentionPrompt(t, since, tail) }
			}
			out = append(out, a)
		}
	}
	return out, nil
}
func expand(s string, values map[string]string) string {
	// One pass: inserted task/comment text is data, never another template.
	var b strings.Builder
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		s = s[i+2:]
		j := strings.Index(s, "}}")
		if j < 0 {
			b.WriteString("{{" + s)
			break
		}
		b.WriteString(values[strings.TrimSpace(s[:j])])
		s = s[j+2:]
	}
	return b.String()
}
