package sidebar

import (
	tea "github.com/charmbracelet/bubbletea"

	"tagent/internal/scrape"
	"tagent/internal/tmux"
)

// agentsKey is the Agents list (the dashboard's first card and the narrow
// sidebar): selection, jump, filter, pin, worktree actions, approvals.
func (m Model) agentsKey(key string) (Model, tea.Cmd, bool) {
	if m.dashboard { // the dashboard card has a log and a prompt box for the agent under the cursor
		if id, ok := paneIDAt(m.rows, m.cursor); ok {
			switch key {
			case "l": // the agent's recent output, without leaving the dashboard
				return m, popup(m.tmuxPane, "tmux capture-pane -p -e -S -300 -t "+shellQuote(id)+" | less -R +G", func() tea.Msg { return nil }), true
			case "i": // type a prompt into the agent
				m.taskForm, m.taskInput, m.promptPane = "i", "", id
				return m, nil, true
			}
		}
	}
	// Approval inbox: on a waiting agent, its prompt's keys answer it.
	if id, ok := paneIDAt(m.rows, m.cursor); ok {
		if d, ok := m.dialogs[id]; ok && m.dialogOpen[id] {
			for _, o := range d.Options {
				if key == o.Key {
					key := o.Key
					delete(m.dialogs, id) // gone once answered; refresh confirms
					m.applyRows()
					return m, func() tea.Msg {
						scrape.Answer(id, key)
						return refreshRequestMsg{}
					}, true
				}
			}
		}
	}

	switch key {
	case "N":
		m.taskForm, m.taskStep = "new-agent", 0
		return m, nil, true
	case "n":
		// Jump to the next agent that needs the user (waiting or error).
		n := paneRowCount(m.rows)
		for i := 1; i <= n; i++ {
			idx := (m.cursor + i) % n
			if r, ok := paneRowAt(m.rows, idx); ok && (r.Status == tmux.StatusWaiting || r.Status == tmux.StatusError) {
				m.cursor, m.cursorDirty = idx, true
				m.syncViewport()
				return m, m.detailRefreshCmd(), true
			}
		}
		return m, nil, true

	case "j", "down":
		if next, moved := moveCursor(m.cursor, 1, paneRowCount(m.rows)); moved {
			m.cursor = next
			m.cursorDirty = true
			m.syncViewport()
			// The bottom summary panel tracks the cursor (part 6, replacing
			// the "i" popup — real user feedback: "show it directly at the
			// root") — moving panes must refetch git/activity RIGHT AWAY,
			// not wait for the periodic tick.
			if _, ok := paneIDAt(m.rows, m.cursor); ok {
				return m, m.detailRefreshCmd(), true
			}
		}
		return m, nil, true

	case "k", "up":
		if next, moved := moveCursor(m.cursor, -1, paneRowCount(m.rows)); moved {
			m.cursor = next
			m.cursorDirty = true
			m.syncViewport()
			if _, ok := paneIDAt(m.rows, m.cursor); ok {
				return m, m.detailRefreshCmd(), true
			}
		}
		return m, nil, true

	case " ":
		next, cmd := m.quickAction()
		return next.(Model), cmd, true

	case "r":
		m.popup.ToggleRepo(m.allRepoNames(), m.repoFilter)
		m.syncViewport()
		return m, nil, true

	case "enter":
		// No row at all (empty list or cursor out of range) does nothing.
		// The jump fires tmux's after-select-pane hook, which clears
		// @pane_unseen (agent-sidebar.conf).
		if paneID, ok := paneIDAt(m.rows, m.cursor); ok {
			// A pending approval: the first Enter shows its answers, the
			// next one jumps to the pane.
			if _, pending := m.dialogs[paneID]; pending && !m.dialogOpen[paneID] {
				m.dialogOpen = map[string]bool{paneID: true}
				m.applyRows()
				return m, nil, true
			}
			return m, selectPaneCmd(paneID, m.tmuxPane), true
		}
		return m, nil, true

	case "v":
		// Toggle multi-select on the pane under the cursor (bulk delete via
		// "x" below). Any row can be selected; ineligible ones (not
		// SidebarSpawned) are simply skipped when "x" collects the list —
		// same silent-ignore behavior as the single-row case.
		if row, ok := paneRowAt(m.rows, m.cursor); ok {
			if m.selectedPanes == nil {
				m.selectedPanes = map[string]bool{}
			}
			if m.selectedPanes[row.PaneID] {
				delete(m.selectedPanes, row.PaneID)
			} else {
				m.selectedPanes[row.PaneID] = true
			}
			m.applyRows()
		}
		return m, nil, true

	case "x":
		// Bulk path: if anything is selected, "x" collects the SELECTED rows
		// eligible for delete (SidebarSpawned) and opens the confirm popup
		// in bulk mode — takes priority over the single-row path below, even
		// if the cursor also happens to sit on an eligible row.
		if len(m.selectedPanes) > 0 {
			var ids []string
			for _, r := range m.rows {
				// Reads m.selectedPanes directly (the source of truth)
				// rather than r.Selected (a copy synced by buildRows) — a
				// pane that was selected then disappeared from the list is
				// simply absent from m.rows, so this loop never has to
				// special-case it.
				if m.selectedPanes[r.PaneID] && r.SidebarSpawned {
					ids = append(ids, r.PaneID)
				}
			}
			if len(ids) > 0 {
				m.removeConfirmOpen = true
				m.removeConfirmPaneIDs = ids
				m.removeConfirmPaneID = ""
				m.removeConfirmBranch = ""
				m.removeConfirmError = ""
				m.syncViewport()
				return m, nil, true
			}
		}
		// Only a row created by the sidebar's own spawn flow (SidebarSpawned)
		// can open the delete confirmation popup — this is the project's
		// only destructive path, silently ignored on a regular row instead
		// of reporting an error (the reference implementation has a flash
		// message here; the Go sidebar has no flash mechanism yet, see the
		// report).
		if row, ok := paneRowAt(m.rows, m.cursor); ok && row.SidebarSpawned {
			m.removeConfirmOpen = true
			m.removeConfirmPaneID = row.PaneID
			m.removeConfirmBranch = row.Branch
			m.removeConfirmError = ""
			m.syncViewport()
		}
		return m, nil, true

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Jump straight to the Nth pane row (1-indexed, matching what the
		// user counts on screen) — same cursor-move-and-refetch as j/k
		// above, just landing directly on an index instead of stepping.
		n := int(key[0] - '1')
		if n < paneRowCount(m.rows) {
			m.cursor = n
			m.cursorDirty = true
			m.syncViewport()
			if _, ok := paneIDAt(m.rows, m.cursor); ok {
				return m, m.detailRefreshCmd(), true
			}
		}
		return m, nil, true

	case "f": // free in the Agents keymap: hand browser-owned sizing back to tmux
		// Only a row the web currently sizes has anything to hand back; on a
		// tmux-owned row the key must do nothing (it would otherwise clear the
		// saved policy and resize a window nobody borrowed).
		if row, ok := paneRowAt(m.rows, m.cursor); ok && m.sizeOwners[row.PaneID] {
			return m, handoffCmd(row.PaneID), true
		}
		return m, nil, true
	case "p":
		// Pin/unpin the pane under the cursor — process-memory only (see
		// Model.pinnedPanes). Pinned panes float to a "Pinned" group right
		// after "Needs you" (buildRows).
		if row, ok := paneRowAt(m.rows, m.cursor); ok {
			if m.pinnedPanes == nil {
				m.pinnedPanes = map[string]bool{}
			}
			if m.pinnedPanes[row.PaneID] {
				delete(m.pinnedPanes, row.PaneID)
			} else {
				m.pinnedPanes[row.PaneID] = true
			}
			m.applyRows()
		}
		return m, nil, true

	case "S":
		// Manually clear a stuck subagent tree — the normal cleanup path
		// (OnSubagentStop) only fires when a subagent finishes NORMALLY; a
		// force-killed or crashed subagent leaves its entry orphaned
		// forever with no other way to clear it. ponytail: a real fix would
		// track per-entry liveness/TTL; this manual escape hatch is the
		// lazy version, upgrade if orphaned entries turn out to be common.
		if row, ok := paneRowAt(m.rows, m.cursor); ok {
			return m, clearSubagentsCmd(row.PaneID), true
		}
		return m, nil, true

	case "/":
		m.filterEditing = true
		return m, nil, true

	case "u":
		// Undo the most recently removed worktree+branch (detail.go:
		// removeCmd/undoRemoveCmd) — single-slot, consumed on use.
		if m.lastRemoved != nil {
			w := *m.lastRemoved
			m.lastRemoved = nil
			return m, tea.Sequence(undoRemoveCmd(w), refreshCmd()), true
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) agentsHints() []hint {
	return []hint{
		{"Tab/S-Tab", "panel", "", ""},
		{"↑↓", "select", "j k ↑ ↓", "select agent"},
		{"enter", "open", "enter", "on ! row: answers; again: jump to pane"},
		{"N", "new agent", "N", "new agent"},
		{"l", "log", "l", "agent log"},
		{"i", "prompt", "i", "send prompt"},
		{"", "", "n", "next agent needing you"}, {"", "", "1-4", "answer open approval"}, {"", "", "space", "quick action on first alert"},
		{"", "", "1-9", "jump to window"}, {"", "", "/", "filter"}, {"", "", "r", "switch repo"}, {"", "", "p", "pin"},
		{"f", "handoff", "f", "handoff sizing to this terminal (web-owned rows only)"}, {"", "", "v x", "select / delete worktrees"}, {"", "", "S", "clear subagents"}, {"", "", "u", "undo"},
	}
}
