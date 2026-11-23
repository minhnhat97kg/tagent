// Package metrics keeps small, prompt-free session and task usage records.
package metrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"tagent/internal/sharedctx"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/usage"
)

type Session struct {
	Key       string           `json:"key"`
	ID        string           `json:"id"`
	Harness   string           `json:"harness"`
	Agent     string           `json:"agent"`
	Pane      string           `json:"pane"`
	Path      string           `json:"path"`
	Status    string           `json:"status"`
	FirstSeen time.Time        `json:"first_seen"`
	LastSeen  time.Time        `json:"last_seen"`
	Usage     usage.Count      `json:"usage"`
	HasUsage  bool             `json:"has_usage"`
	Daily     map[string]int64 `json:"daily,omitempty"`
}

type Run struct {
	TaskID     string    `json:"task_id"`
	SessionKey string    `json:"session_key"`
	Agent      string    `json:"agent"`
	Started    time.Time `json:"started"`
	Ended      time.Time `json:"ended,omitempty"`
	StartTotal int64     `json:"start_total"`
	EndTotal   int64     `json:"end_total"`
	HasStart   bool      `json:"has_start"`
	HasEnd     bool      `json:"has_end"`
}

func (r Run) Tokens() (int64, bool) {
	if !r.HasStart || !r.HasEnd {
		return 0, false
	}
	return max(0, r.EndTotal-r.StartTotal), true
}

// Elapsed is how long the run took (until Ended, or until now while open).
func (r Run) Elapsed(now time.Time) time.Duration {
	end := r.Ended
	if end.IsZero() {
		end = now
	}
	return max(0, end.Sub(r.Started))
}

// RunsOf lists a task's runs, oldest first.
func RunsOf(s State, id string) []Run {
	var out []Run
	for _, r := range s.Runs {
		if r.TaskID == id {
			out = append(out, r)
		}
	}
	return out
}

type State struct {
	Sessions map[string]Session `json:"sessions"`
	Runs     []Run              `json:"runs"`
}

func path(scope string) string { return filepath.Join(sharedctx.Dir(scope), "metrics", "state.json") }
func Read(scope string) State {
	s := State{Sessions: map[string]Session{}}
	b, err := os.ReadFile(path(scope))
	if err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.Sessions == nil {
		s.Sessions = map[string]Session{}
	}
	return s
}
func update(scope string, fn func(*State) error) error {
	dir := filepath.Dir(path(scope))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	s := Read(scope)
	if err = fn(&s); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path(scope))
}
func Key(p tmux.PaneInfo) string {
	if p.SessionID == "" || p.Agent == "" {
		return ""
	}
	return string(p.Agent) + ":" + p.SessionID
}
func findPane(sessions []tmux.SessionInfo, pane string) (tmux.PaneInfo, bool) {
	for _, sess := range sessions {
		for _, w := range sess.Windows {
			for _, p := range w.Panes {
				if p.PaneID == pane {
					return p, true
				}
			}
		}
	}
	return tmux.PaneInfo{}, false
}

// Refresh records live sessions and usage deltas. The first observation is
// a baseline: earlier tokens are shown in the session total, not dated today.
func RefreshOrRead(scope string, sessions []tmux.SessionInfo) State {
	state, err := Refresh(scope, sessions)
	if err != nil {
		return Read(scope)
	}
	return state
}

func Refresh(scope string, sessions []tmux.SessionInfo) (State, error) {
	now := time.Now()
	observed := map[string]bool{}
	err := update(scope, func(s *State) error {
		for _, sess := range sessions {
			for _, w := range sess.Windows {
				for _, p := range w.Panes {
					if p.Path == "" || tasks.Scope(p.Path) != scope {
						continue
					}
					key := Key(p)
					if key == "" {
						continue
					}
					observed[key] = true
					rec, exists := s.Sessions[key]
					if !exists {
						rec = Session{Key: key, ID: p.SessionID, Harness: string(p.Agent), FirstSeen: now, Daily: map[string]int64{}}
					}
					rec.Agent, rec.Pane, rec.Path, rec.Status, rec.LastSeen = p.Name, p.PaneID, p.Path, string(p.Status), now
					if rec.Agent == "" {
						rec.Agent = string(p.Agent)
					}
					if c, ok := usage.Read(string(p.Agent), p.SessionID); ok {
						if rec.Daily == nil {
							rec.Daily = map[string]int64{}
						}
						if exists && rec.HasUsage && c.Total > rec.Usage.Total {
							rec.Daily[now.Format("2006-01-02")] += c.Total - rec.Usage.Total
						}
						if !rec.HasUsage || c.Total >= rec.Usage.Total {
							rec.Usage, rec.HasUsage = c, true
						}
						for i := range s.Runs {
							if s.Runs[i].SessionKey == key && s.Runs[i].Ended.IsZero() {
								s.Runs[i].EndTotal, s.Runs[i].HasEnd = c.Total, true
							}
						}
					}
					s.Sessions[key] = rec
				}
			}
		}
		for key, rec := range s.Sessions {
			if !observed[key] && rec.Status != "closed" {
				rec.Status = "closed"
				s.Sessions[key] = rec
				for i := range s.Runs {
					if s.Runs[i].SessionKey == key && s.Runs[i].Ended.IsZero() {
						s.Runs[i].Ended = now
					}
				}
			}
		}
		return nil
	})
	return Read(scope), err
}

func Link(scope, taskID string, p tmux.PaneInfo) error {
	if _, err := tasks.Get(scope, taskID); err != nil {
		return err
	}
	if p.Path == "" || tasks.Scope(p.Path) != scope {
		return errors.New("agent is outside this repo")
	}
	key := Key(p)
	if key == "" {
		return errors.New("agent has no native session id yet")
	}
	count, ok := usage.Read(string(p.Agent), p.SessionID)
	return update(scope, func(s *State) error {
		for _, r := range s.Runs {
			if r.SessionKey == key && r.Ended.IsZero() {
				if r.TaskID == taskID {
					return nil
				}
				return fmt.Errorf("session is already working on task %s", r.TaskID)
			}
		}
		name := p.Name
		if name == "" {
			name = string(p.Agent)
		}
		if _, exists := s.Sessions[key]; !exists {
			s.Sessions[key] = Session{Key: key, ID: p.SessionID, Harness: string(p.Agent), Agent: name, Pane: p.PaneID, Path: p.Path, Status: string(p.Status), FirstSeen: time.Now(), LastSeen: time.Now(), Usage: count, HasUsage: ok, Daily: map[string]int64{}}
		}
		s.Runs = append(s.Runs, Run{TaskID: taskID, SessionKey: key, Agent: name, Started: time.Now(), StartTotal: count.Total, EndTotal: count.Total, HasStart: ok, HasEnd: ok})
		return nil
	})
}

func End(scope, taskID string) error {
	return update(scope, func(s *State) error {
		for i := range s.Runs {
			r := &s.Runs[i]
			if r.TaskID != taskID || !r.Ended.IsZero() {
				continue
			}
			if sess, ok := s.Sessions[r.SessionKey]; ok {
				if c, found := usage.Read(sess.Harness, sess.ID); found {
					r.EndTotal, r.HasEnd = c.Total, true
				}
			}
			r.Ended = time.Now()
		}
		return nil
	})
}
func TaskTokens(s State, id string) (int64, int) {
	var total int64
	missing := 0
	for _, r := range s.Runs {
		if r.TaskID == id {
			if n, ok := r.Tokens(); ok {
				total += n
			} else {
				missing++
			}
		}
	}
	return total, missing
}
func ActiveTask(s State, key string) string {
	for _, r := range s.Runs {
		if r.SessionKey == key && r.Ended.IsZero() {
			return r.TaskID
		}
	}
	return ""
}
func SessionList(s State) []Session {
	list := make([]Session, 0, len(s.Sessions))
	for _, v := range s.Sessions {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].LastSeen.After(list[j].LastSeen) })
	return list
}
func Clear(scope string) error {
	if strings.TrimSpace(scope) == "" {
		return errors.New("empty scope")
	}
	return os.Remove(path(scope))
}
