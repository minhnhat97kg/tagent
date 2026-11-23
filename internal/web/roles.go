package web

import (
	"net/http"

	"tagent/internal/agents"
	"tagent/internal/tmux"
)

// roles lists, saves and deletes agents (GET, POST, DELETE /api/roles). POST /api/agents is spawn.
func (s *server) roles(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	harness, _ := tmux.GetOption(tmux.AgentOption)
	if r.Method == "GET" {
		writeJSON(w, agents.List(dir, harness, agents.LivePanes()))
		return
	}
	var err error
	if r.Method == "DELETE" {
		var o struct{ Name string }
		if err = readBody(r, &o); err == nil {
			err = agents.Delete(dir, harness, o.Name)
		}
	} else {
		var a agents.Agent
		if err = readBody(r, &a); err == nil {
			err = agents.Save(dir, harness, a)
		}
	}
	if err != nil {
		fail(w, err)
	}
}
