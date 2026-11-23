package web

import (
	"net/http"
	"time"

	"tagent/internal/tasks"
)

// runStep is the compact flow strip: one card per workflow job.
type runStep struct {
	Job     string `json:"job"`
	Status  string `json:"status"`
	Count   int    `json:"count"`
	Current bool   `json:"current"`
}

// runView is what actually ran for one ticket: no YAML, no workflow list.
type runView struct {
	Ticket   string            `json:"ticket"`
	Title    string            `json:"title"`
	Workflow string            `json:"workflow"`
	Inputs   map[string]string `json:"inputs,omitempty"`
	Status   string            `json:"status"`
	Started  time.Time         `json:"started"`
	Total    time.Duration     `json:"total"`
	Entries  []tasks.RunEntry  `json:"entries"`
	Flow     []runStep         `json:"flow"`
}

func (s *server) run(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	scope := tasks.Scope(dir)
	t, err := tasks.Get(scope, r.PathValue("id"))
	if err != nil {
		http.Error(w, "ticket not found", http.StatusNotFound)
		return
	}
	if t.Workflow == "" {
		http.Error(w, "ticket has no workflow", http.StatusNotFound)
		return
	}
	base, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		fail(w, err)
		return
	}
	cfg, err := tasks.ConfigForTask(dir, base, t)
	if err != nil {
		fail(w, err)
		return
	}
	events, err := tasks.ReadEvents(scope, 0)
	if err != nil {
		fail(w, err)
		return
	}
	now := time.Now().UTC()
	entries := tasks.BuildRuns(t, cfg, events, now)
	v := runView{Ticket: t.ID, Title: t.Title, Workflow: t.Workflow, Inputs: t.Inputs, Status: t.Status, Entries: entries, Flow: []runStep{}}
	if len(entries) > 0 {
		end := now
		if last := entries[len(entries)-1]; last.End != nil {
			end = *last.End
		}
		v.Started, v.Total = entries[0].Start, end.Sub(entries[0].Start)
	}
	count := map[string]int{}
	for _, e := range entries {
		count[e.Step]++
	}
	for _, st := range cfg.Statuses {
		if st.Job != "" {
			v.Flow = append(v.Flow, runStep{Job: st.Job, Status: st.ID, Count: count[st.ID], Current: st.ID == t.Status && len(entries) > 0 && entries[len(entries)-1].End == nil})
		}
	}
	writeJSON(w, v)
}
