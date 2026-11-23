package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"tagent/internal/intake"
	"tagent/internal/metrics"
	"tagent/internal/quota"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"time"
)

type apiDeps struct {
	sessions func() []tmux.SessionInfo
	refresh  func(string, []tmux.SessionInfo) metrics.State
	quota    func() quota.Snapshot
	command  func() string
	harness  *intake.Harness
	execute  func(string, string) (string, error)
}

func (s *server) sessions() []tmux.SessionInfo {
	if s.deps.sessions != nil {
		return s.deps.sessions()
	}
	return tmux.QuerySessions()
}
func validDir(dir string) error {
	st, e := os.Stat(dir)
	if e != nil || !st.IsDir() {
		return fmt.Errorf("no directory %q", dir)
	}
	return nil
}
func (s *server) overview(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	if e := validDir(dir); e != nil {
		fail(w, e)
		return
	}
	scope := tasks.Scope(dir)
	sessions := s.sessions()
	refresh := s.deps.refresh
	if refresh == nil {
		refresh = metrics.RefreshOrRead
	}
	state := refresh(scope, sessions)
	panes := []tmux.PaneInfo{}
	seen := map[string]bool{}
	for _, se := range sessions {
		for _, wi := range se.Windows {
			for _, p := range wi.Panes {
				if p.Agent != "" && !seen[p.PaneID] && tasks.Scope(p.Path) == scope {
					seen[p.PaneID] = true
					panes = append(panes, p)
				}
			}
		}
	}
	attention, now := metrics.OverviewFacts(panes, tasks.List(scope), state)
	get := s.deps.quota
	if get == nil {
		get = quota.Get
	}
	q := get()
	type provider struct {
		Name  string   `json:"name"`
		Used  *float64 `json:"usedPercent"`
		Week  *float64 `json:"weeklyPercent"`
		Stale bool     `json:"stale"`
	}
	writeJSON(w, map[string]any{"dir": dir, "project": filepath.Base(tasks.MainRoot(dir)), "attention": attention, "now": now, "providers": []provider{{"Claude", q.Claude.Five.Used, q.Claude.Week.Used, q.Claude.Stale}, {"Codex", q.Codex.Five.Used, q.Codex.Week.Used, q.Codex.Stale}}, "tokens": metrics.TokenWindow(state, time.Now(), 7)})
}
