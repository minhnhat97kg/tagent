package board

import (
	"slices"
	"strings"

	"tagent/internal/tasks"
)

// LivePanes reports, for a role on this board, each ticket that has a pane of
// that role and whether the pane is idle. Idle panes hold no slot. An error
// means the panes could not be read, so the dispatch must wait, not fire.
type LivePanes func(dir, role string) (map[string]bool, error)

// waiting is a pane dispatch held back because its role is at max_instances.
// The event is kept so the dispatch can be re-derived after a restart.
type waiting struct {
	Seq     int64       `json:"seq"`
	Trigger int         `json:"trigger"`
	Role    string      `json:"role"`
	Event   tasks.Event `json:"event"`
}

// slotFull returns the role's limit and its running tickets when a's pane dispatch
// must wait. A ticket that already has a pane of the role reuses it and needs no
// slot; run jobs, long-lived panes and roles without max_instances never wait.
// A read error is returned with max 0; the caller must then wait.
func slotFull(dir string, cfg tasks.BoardConfig, a Action, ticket string, live LivePanes) (int, []string, error) {
	r, ok := cfg.Roles[a.Agent]
	if live == nil || !ok || r.MaxInstances <= 0 || a.Run != "" || a.LongLived || a.Agent == "" {
		return 0, nil, nil
	}
	panes, err := live(dir, a.Agent)
	if err != nil {
		return 0, nil, err
	}
	if _, own := panes[ticket]; own {
		return 0, nil, nil
	}
	var running []string
	for t, idle := range panes {
		if !idle {
			running = append(running, t)
		}
	}
	if len(running) < r.MaxInstances {
		return 0, nil, nil
	}
	slices.Sort(running)
	return r.MaxInstances, running, nil
}

func ticketList(ids []string) string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = "#" + id
	}
	return strings.Join(out, ", ")
}
