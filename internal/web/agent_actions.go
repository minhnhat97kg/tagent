package web

import (
	"fmt"
	"net/http"
	"strings"
	"tagent/internal/activity"
	"tagent/internal/tmux"
	"tagent/internal/worktree"
)

func (s *server) agentAction(w http.ResponseWriter, r *http.Request) {
	p, ok := s.paneBody(w, r)
	if !ok {
		return
	}
	switch p.Data {
	case "pin":
		if _, ok := s.pinned.Load(p.Pane); ok {
			s.pinned.Delete(p.Pane)
		} else {
			s.pinned.Store(p.Pane, true)
		}
	case "delete-worktree":
		if err := worktree.Remove(p.Pane, worktree.RemoveWindowAndWorktree); err != nil {
			fail(w, err)
		}
	case "log":
		var text strings.Builder
		for _, e := range activity.ReadActivityLog(p.Pane, 100) {
			fmt.Fprintf(&text, "%s  %s  %s\n", e.Timestamp, e.Tool, e.Label)
		}
		writeJSON(w, map[string]string{"text": text.String()})
	case "undo":
		out, err := self(p.Data, p.Pane)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]string{"text": out})
	case "dismiss":
		tmux.UnsetPaneOption(p.Pane, tmux.PaneUnseen)
	case "clear-subagents":
		tmux.UnsetPaneOption(p.Pane, tmux.PaneSubagents)
	default:
		fail(w, fmt.Errorf("unknown agent action"))
	}
}
