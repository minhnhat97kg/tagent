package web

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

type ticketChatJSON struct {
	Key       string `json:"key"`
	Pane      string `json:"pane"`
	Harness   string `json:"harness"`
	Status    string `json:"status"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Created   bool   `json:"created,omitempty"`
}

// ticketChat resolves a ticket's chat pane. GET only reports it (pane is empty
// until the chat starts); POST starts or reuses it through `tagent chat`, which
// owns the spawn path, the @pane_ticket binding and the first-message context.
// Messages themselves keep using /api/pane/send and the transcript endpoint.
func (s *server) ticketChat(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	if err := validDir(dir); err != nil {
		fail(w, err)
		return
	}
	id := r.PathValue("id")
	scope := tasks.Scope(dir)
	if _, err := tasks.Get(scope, id); err != nil {
		http.Error(w, "ticket not found", http.StatusNotFound)
		return
	}
	key := tasks.TicketScope(dir, id)
	out := ticketChatJSON{Key: key, Available: true}
	panes, err := tmux.TicketPanes()
	if err != nil {
		fail(w, err)
		return
	}
	if p, ok := findTicketPane(panes, key); ok {
		out.Pane, out.Harness, out.Status = p.PaneID, p.Agent, string(p.Status)
		if !ticketChatHarness(p.Agent) {
			out.Available = false
			out.Reason = fmt.Sprintf("Chat needs a claude or pi agent, but %s runs %s.", p.PaneID, p.Agent)
		}
	}
	if r.Method != http.MethodPost {
		writeJSON(w, out)
		return
	}
	var o struct {
		Agent string
		Clear bool
	}
	if r.ContentLength != 0 {
		if err := readBody(r, &o); err != nil {
			fail(w, err)
			return
		}
	}
	unlock := s.lockTicketChat(key)
	defer unlock()
	before := out.Pane
	args := []string{"chat", "--dir", dir}
	if o.Agent != "" {
		args = append(args, "--agent", o.Agent)
	}
	if o.Clear {
		args = append(args, "--clear")
	}
	args = append(args, id)
	pane, err := self(args...)
	if err != nil {
		fail(w, err)
		return
	}
	pane = strings.Fields(pane + " ")[0]
	out.Pane = pane
	out.Created = o.Clear || pane != before
	out.Harness = tmux.GetPaneOptionValue(pane, tmux.PaneAgent)
	out.Status = string(tmux.ParsePaneStatus(tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt)))
	out.Available, out.Reason = true, ""
	writeJSON(w, out)
}

func (s *server) lockTicketChat(key string) func() {
	v, _ := s.chatLocks.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func ticketChatHarness(agent string) bool {
	return agent == "" || agent == "claude" || agent == "pi"
}

func findTicketPane(panes []tmux.TicketPane, key string) (tmux.TicketPane, bool) {
	var found tmux.TicketPane
	ok := false
	for _, p := range panes {
		if p.Ticket != key {
			continue
		}
		if !ok || (ticketChatHarness(p.Agent) && !ticketChatHarness(found.Agent)) {
			found, ok = p, true
		}
	}
	return found, ok
}
