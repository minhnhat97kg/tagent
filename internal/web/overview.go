package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"tagent/internal/harnessusage"
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
func (s *server) quota() quota.Snapshot {
	if s.deps.quota != nil {
		return s.deps.quota()
	}
	return quota.Get()
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
	q := s.quota()
	type provider struct {
		Name  string   `json:"name"`
		Used  *float64 `json:"usedPercent"`
		Week  *float64 `json:"weeklyPercent"`
		Stale bool     `json:"stale"`
	}
	days := 7
	switch r.URL.Query().Get("days") {
	case "1":
		days = 1
	case "30":
		days = 30
	}
	writeJSON(w, map[string]any{"dir": dir, "project": filepath.Base(tasks.MainRoot(dir)), "attention": attention, "now": now, "providers": []provider{{"Claude", q.Claude.Five.Used, q.Claude.Week.Used, q.Claude.Stale}, {"Codex", q.Codex.Five.Used, q.Codex.Week.Used, q.Codex.Stale}}, "harnesses": harnessusage.Rows(), "tokens": metrics.TokenWindow(state, time.Now(), days)})
}
