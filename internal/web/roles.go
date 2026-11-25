package web

import (
	"encoding/json"
	"io"
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
		var raw []byte
		raw, err = io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err == nil {
			err = json.Unmarshal(raw, &a)
		}
		if err == nil {
			a.ContextSet = contextSent(raw)
			err = agents.Save(dir, harness, a)
		}
	}
	if err != nil {
		fail(w, err)
	}
}

// contextSent reports whether a role save body carries any context key.
func contextSent(raw []byte) bool {
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil {
		return false
	}
	for _, k := range []string{"skills", "mcp", "tools", "allow", "deny"} {
		if _, ok := keys[k]; ok {
			return true
		}
	}
	return false
}
