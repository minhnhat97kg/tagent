package sidebar

import (
	tea "github.com/charmbracelet/bubbletea"

	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

// configKey is the board config view (B).
func (m Model) configKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "esc", "w", "q", "b", "B":
		m.boardFlow, m.boardConfigView = false, false
	case "j", "down":
		m.boardConfigScroll = min(m.boardConfigScroll+1, m.boardConfigMaxScroll())
	case "k", "up":
		m.boardConfigScroll = max(0, m.boardConfigScroll-1)
	case "g":
		m.boardConfigScroll = 0
	case "G":
		m.boardConfigScroll = m.boardConfigMaxScroll()
	case "enter":
		m.taskForm, m.taskStep = "default", len(m.boardWorkflows)
		for i, w := range m.boardWorkflows {
			if w.Name == m.boardConfig.DefaultWorkflow {
				m.taskStep = i
			}
		}
		return m, nil, true
	case "e", "E":
		return m, editBoardConfig(m.boardDir, m.tmuxPane), true
	default:
		return m, nil, false
	}
	return m, nil, true
}

// flowKey is the workflow view (w).
func (m Model) flowKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "[", "]":
		ws := workflow.List(tasks.MainRoot(m.boardDir))
		if len(ws) > 0 {
			i := -1
			for n, w := range ws {
				if w.File == m.flowWorkflow || (m.flowWorkflow == "" && len(m.notes) > 0 && w.Name == m.notes[m.taskCursor].Workflow) {
					i = n
					break
				}
			}
			if i < 0 {
				if key == "]" {
					i = len(ws) - 1
				} else {
					i = 0
				}
			}
			if key == "]" {
				i = (i + 1) % len(ws)
			} else {
				i = (i + len(ws) - 1) % len(ws)
			}
			m.flowWorkflow = ws[i].File
		}
		return m, nil, true
	case "e", "E":
		path := m.flowWorkflow
		if path == "" && len(m.notes) > 0 && m.notes[m.taskCursor].Workflow != "" {
			w, e := workflow.Find(tasks.MainRoot(m.boardDir), m.notes[m.taskCursor].Workflow)
			if e != nil {
				m.taskNotice = e.Error()
				return m, nil, true
			}
			path = w.File
		}
		if path != "" {
			dir := m.boardDir
			return m, editFile(m.tmuxPane, path, func() tea.Msg { return reloadBoardConfig(dir, nil) }), true
		}
		return m, editBoardConfig(m.boardDir, m.tmuxPane), true
	case "esc", "w", "q":
		m.boardFlow = false
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) flowHints() []hint {
	return []hint{
		{"esc", "close", "esc w q", "close workflow view"}, {"[/]", "browse", "[ ]", "browse workflows"}, {"e", "edit", "e E", "edit displayed workflow (board config if none)"},
	}
}

func (m Model) configHints() []hint {
	return []hint{
		{"esc", "close", "esc w q B", "close"}, {"enter", "default", "enter", "pick the default workflow (saved to board.yaml)"}, {"j/k", "scroll", "j k ↑ ↓", "scroll default workflow, roles, triggers"},
		{"E", "edit", "e E", "edit .tasks/board.yaml in $EDITOR"}, {"", "", "g G", "top / bottom"}, {"", "", "B", "open from any card"},
	}
}
