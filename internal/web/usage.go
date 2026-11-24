package web

import (
	"net/http"
	"time"

	"tagent/internal/metrics"
	"tagent/internal/tasks"
)

// ticketUsage is one ticket's token usage per job and model. Unlike the run, it
// works for tickets without a workflow, and refreshes so a running ticket is live.
func (s *server) ticketUsage(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	scope := tasks.Scope(dir)
	t, err := tasks.Get(scope, r.PathValue("id"))
	if err != nil {
		http.Error(w, "ticket not found", http.StatusNotFound)
		return
	}
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		fail(w, err)
		return
	}
	if c, err := tasks.ConfigForTask(dir, cfg, t); err == nil {
		cfg = c
	}
	events, err := tasks.ReadEvents(scope, 0)
	if err != nil {
		fail(w, err)
		return
	}
	refresh := s.deps.refresh
	if refresh == nil {
		refresh = metrics.RefreshOrRead
	}
	u := metrics.TicketUsageOf(refresh(scope, s.sessions()), t, cfg, events, time.Now().UTC())
	if u.Jobs == nil {
		u.Jobs, u.Models = []metrics.JobUsage{}, []metrics.ModelUsage{}
	}
	writeJSON(w, u)
}
