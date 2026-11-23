package tmux

import "strings"

// TicketPane is a pane bound to a per-ticket chat.
type TicketPane struct {
	PaneID string
	Ticket string // "<project scope>#<ticket id>", see tasks.TicketScope
	Agent  string
	Status PaneStatus
	Path   string
}

// TicketPanes lists panes bound to a ticket with @pane_ticket. It deliberately
// does not run agent detection or cleanup: a chat pane is listed from the
// moment it is spawned, before its hook reports an agent.
func TicketPanes() ([]TicketPane, error) {
	out, err := RunTmuxCapture("list-panes", "-a", "-F",
		"#{pane_id}|#{q:"+PaneTicket+"}|#{q:"+PaneAgent+"}|#{q:"+PaneStatusOpt+"}|#{q:pane_current_path}")
	if err != nil {
		return nil, err
	}
	var panes []TicketPane
	for _, line := range strings.Split(out, "\n") {
		p := splitTmuxFields(line, '|')
		if len(p) != 5 || p[1] == "" {
			continue
		}
		panes = append(panes, TicketPane{
			PaneID: p[0],
			Ticket: p[1],
			Agent:  p[2],
			Status: ParsePaneStatus(p[3]),
			Path:   p[4],
		})
	}
	return panes, nil
}
