package tasks

import (
	"os"
	"strings"

	"tagent/internal/tmux"
)

// TicketScope is the chat scope of one ticket: the project scope and the
// ticket id joined with '#', for example "tagent-1a2b3c4d#12". Ticket ids
// are numbered per repository, so the key is unique and stable. It is stored
// in the pane option @pane_ticket and returned by the web chat API.
func TicketScope(dir, id string) string { return Scope(dir) + "#" + id }

// CallerTicket is the ticket the current pane is bound to ("scope#id").
// ok is false outside tmux and for panes not started as a ticket chat.
func CallerTicket() (scope, id string, ok bool) {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return "", "", false
	}
	scope, id, ok = strings.Cut(tmux.GetPaneOptionValue(pane, tmux.PaneTicket), "#")
	if !ok || scope == "" || id == "" {
		return "", "", false
	}
	return scope, id, true
}

// ContextFor is the brief a per-ticket assistant starts from: the ticket summary
// (Brief), its workflow step and the next statuses. Details are one
// `tagent task show` away, so the full description and comment history are not
// injected here.
func ContextFor(scope, id string) (string, error) {
	t, err := Get(scope, id)
	if err != nil {
		return "", err
	}
	cfg, err := configForTaskScope(scope, t)
	if err != nil {
		return "", err
	}
	step := ""
	for _, s := range cfg.Statuses {
		if s.ID != t.Status {
			continue
		}
		step = s.ID
		if s.Role != "" {
			step += " (role " + s.Role + ")"
		}
		if s.Job != "" {
			step += " (job " + s.Job + ")"
		}
		if len(s.Next) > 0 {
			step += "; next: " + strings.Join(s.Next, ", ")
		}
	}
	return Brief(t) + "Step: " + step + "\nFor details run `tagent task show " + t.ID + "`.\n", nil
}
