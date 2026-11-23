package sidebar

import tea "github.com/charmbracelet/bubbletea"

// detailKey is the task detail drawer: its tabs and scrolling, then the
// ticket keys (edit, comment, priority...) that stay live while it is open.
func (m Model) detailKey(key string) (Model, tea.Cmd, bool) {
	if m.detailTabKey(key) {
		return m, nil, true
	}
	switch key {
	case "s": // the dispatch noise stays folded until asked for
		if m.detailTab == detailTabActivity {
			m.detailSysOpen = !m.detailSysOpen
		}
		return m, nil, true
	case "enter", " ": // expand a long description in place, or start the ticket's chat
		switch m.detailTab {
		case detailTabDetails:
			m.detailDescOpen = !m.detailDescOpen
		case detailTabChat:
			if m.chat.Pane == "" && m.chat.available() && len(m.notes) > 0 {
				dir, id := m.boardDir, m.notes[m.taskCursor].ID
				m.taskNotice = "Starting the chat…"
				return m, startTicketChat(dir, id), true
			}
		}
		return m, nil, true
	case "i": // message the chat's agent; the form's footer says how to save it
		if m.detailTab == detailTabChat {
			if m.chat.started() {
				m.taskForm, m.taskInput = "chat", ""
			}
			return m, nil, true
		}
	case "c", "C": // the Activity tab's inline composer
		if m.detailTab == detailTabActivity {
			return m.commentBoxOpen(), nil, true
		}
	case "S": // save the newest reply to the ticket as a comment
		if m.detailTab == detailTabChat {
			return m.saveChatReply(), nil, true
		}
	}
	if m.scrollDetail(key) {
		return m, nil, true
	}
	return m.ticketsKey(key)
}
