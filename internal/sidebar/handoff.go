package sidebar

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"tagent/internal/tmux"
)

// HandoffPane returns a browser-sized window to tmux's attached terminals.
func HandoffPane(pane string) error {
	policy := tmux.GetPaneOptionValue(pane, "@agent_size_policy")
	if _, ok := tmux.RunTmux("set-option", "-p", "-t", pane, "@agent_size_owner", ""); !ok {
		return fmt.Errorf("cannot clear size owner")
	}
	restore := func() bool {
		if strings.HasPrefix(policy, "value:") {
			_, ok := tmux.RunTmux("set-option", "-w", "-t", pane, "window-size", strings.TrimPrefix(policy, "value:"))
			return ok
		}
		if policy == "inherit" {
			_, ok := tmux.RunTmux("set-option", "-w", "-u", "-t", pane, "window-size")
			return ok
		}
		return true
	}
	// Undo the zoom the web applied to give this pane the whole window.
	if tmux.GetPaneOptionValue(pane, "@agent_size_zoomed") == "1" {
		if strings.TrimSpace(tmux.DisplayMessage(pane, "#{window_zoomed_flag}")) == "1" {
			tmux.RunTmux("resize-pane", "-Z", "-t", pane)
		}
		tmux.UnsetPaneOption(pane, "@agent_size_zoomed")
	}
	first := restore()
	_, ok := tmux.RunTmux("resize-window", "-A", "-t", pane)
	last := restore()
	// -A can leave a manual policy when no saved policy exists. Use tmux's inherited policy.
	if policy == "" {
		tmux.RunTmux("set-option", "-w", "-u", "-t", pane, "window-size")
	}
	tmux.UnsetPaneOption(pane, "@agent_size_policy")
	if !first || !ok || !last {
		return fmt.Errorf("cannot restore terminal sizing")
	}
	return nil
}

// handoffDoneMsg reports the ownership state tmux has after a handoff key, so
// the list can stop drawing the row as "sized by web" right away: reading it
// again inside Update would block the UI on a subprocess, and readSizeOwners
// only runs on the periodic refresh tick (refreshInterval), which left the
// label stale for seconds after the key — the ticket-42 report read that as
// "no way to hand off".
type handoffDoneMsg struct {
	pane string
	web  bool // still owned by a browser: the release did not take effect
}

func handoffCmd(pane string) tea.Cmd {
	return func() tea.Msg {
		_ = HandoffPane(pane)
		return handoffDoneMsg{pane: pane, web: strings.HasPrefix(tmux.GetPaneOptionValue(pane, "@agent_size_owner"), "web:")}
	}
}

func readSizeOwners(sessions []tmux.SessionInfo) map[string]bool {
	out := map[string]bool{}
	for _, s := range sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.Agent != "" {
					out[p.PaneID] = strings.HasPrefix(tmux.GetPaneOptionValue(p.PaneID, "@agent_size_owner"), "web:")
				}
			}
		}
	}
	return out
}
