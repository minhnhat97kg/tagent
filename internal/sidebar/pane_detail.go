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
	case "c", "C": // the inline composer lives on Activity; open it from any tab
		m.detailTab = detailTabActivity
		return m.commentBoxOpen(), nil, true
	case "S": // save the newest reply to the ticket as a comment
		if m.detailTab == detailTabChat {
			return m.saveChatReply(), nil, true
		}
	}
	if m.scrollDetail(key) {
		return m, nil, true
	}
	switch key { // the ticket keys that stay live under the drawer; the others act on the lanes behind it
	case "esc", "enter", "i", "e", "p", "D", "a", "A", "q", "b":
		return m.ticketsKey(key)
	}
	return m, nil, false
}

// tabKey is key when the drawer is on tab (so it shows in the footer), else "".
func (m Model) tabKey(tab int, key string) string {
	if m.detailTab == tab {
		return key
	}
	return ""
}

func (m Model) detailHints() []hint {
	return []hint{
		{"esc", "close", "esc", "close detail drawer"}, {"tab/1-6", "tabs", "tab h l 1-6", "switch Details / Files / Needs / Chat / Activity / Run"}, {"j/k", "scroll", "j k ↑ ↓", "scroll task detail"}, {"e", "edit", "e", "edit task"},
		{"c", "comment", "c", "comment box on Activity (@ suggests roles, enter posts, esc cancels)"},
		{m.tabKey(detailTabActivity, "s"), "events", "s", "Activity: show / hide the dispatch messages"}, {"", "", "pgup pgdn", "scroll page"}, {"", "", "g G", "top / bottom"}, {"", "", "enter i", "toggle detail drawer; Chat: start the chat"}, {m.tabKey(detailTabChat, "i"), "message", "i", "Chat: message the agent"}, {m.tabKey(detailTabChat, "S"), "save reply", "S", "Chat: save its reply to the ticket"},
		{"", "", "p / D", "cycle priority / edit dependencies"}, {"", "", "c / C", "blocked: @role reply resumes; C: comment"}, {"", "", "a / A", "view attachments / attach file"},
	}
}
