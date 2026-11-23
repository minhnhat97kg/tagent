package sidebar

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
)

type workflowsEditedMsg struct{ err error }

// editFile opens path in $EDITOR in a tmux popup over pane. The sidebar runs in a
// shared server with no terminal, so ExecProcess would start an invisible editor
// and freeze the UI waiting for it; popup only takes the screen with no pane.
func editFile(pane, path string, done func() tea.Msg) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	return popup(pane, editor+" "+shellQuote(path), done)
}

func editWorkflowFile(pane, path string) tea.Cmd {
	return editFile(pane, path, func() tea.Msg { return workflowsEditedMsg{} })
}

// Create exclusively: even a concurrent creator must never lose an existing recipe.
func createWorkflowFile(root, name string) (string, error) {
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("workflow name required")
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return "", fmt.Errorf("use letters, digits, - or _ in workflow names")
		}
	}
	dir := workflow.LocalDir(tasks.MainRoot(root))
	if e := os.MkdirAll(dir, 0755); e != nil {
		return "", e
	}
	path := filepath.Join(dir, name+".yml")
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return "", e
	}
	_, e = f.WriteString(workflow.Template(name))
	ce := f.Close()
	if e == nil {
		e = ce
	}
	return path, e
}
func (m Model) workflowPanelKey(key string) (Model, tea.Cmd, bool) {
	if m.agentsOpen {
		return m.agentPanelKey(key)
	}
	if m.workflowDelete != "" {
		switch key {
		case "y":
			path := m.workflowDelete
			m.workflowDelete = ""
			return m, func() tea.Msg { return workflowsEditedMsg{os.Remove(path)} }, true
		case "n", "esc":
			m.workflowDelete = ""
		}
		return m, nil, true
	}
	switch key {
	case "tab", "shift+tab":
		m.workflowFocus = false
	case "j", "down":
		m.workflowCursor = min(max(0, len(m.boardWorkflows)-1), m.workflowCursor+1)
	case "k", "up":
		m.workflowCursor = max(0, m.workflowCursor-1)
	case "a":
		m.taskForm = "workflow-new"
		m.taskInput = ""
	case "t": // swap this card for agents (same as t from the tickets)
		m.agentsOpen, m.agentDetail, m.agentCursor = true, false, 0
		m = m.loadAgents()
	case "B": // board config summary (same view as the board's B key)
		m.workflowFocus = false
		m.boardFlow, m.boardConfigView = true, true
		m.flowWorkflow, m.boardConfigScroll = "", 0
	case "esc", "q", "b":
		m.workflowFocus = false
		m.boardMode = false
	case "ctrl+c":
		return m, tea.Quit, true
	case "enter", "w", "e", "x":
		if len(m.boardWorkflows) == 0 {
			return m, nil, true
		}
		m.workflowCursor = max(0, min(m.workflowCursor, len(m.boardWorkflows)-1))
		w := m.boardWorkflows[m.workflowCursor]
		switch key {
		case "enter", "w":
			m.boardFlow = true
			m.flowWorkflow = w.File
		case "e":
			return m, editWorkflowFile(m.tmuxPane, w.File), true
		case "x":
			m.workflowDelete = w.File
		}
	}
	return m, nil, true
}
func (m Model) handleWorkflowName(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyEsc:
		m.taskForm = ""
	case tea.KeyBackspace, tea.KeyDelete:
		r := []rune(m.taskInput)
		if len(r) > 0 {
			m.taskInput = string(r[:len(r)-1])
		}
	case tea.KeyRunes:
		m.taskInput += string(key.Runes)
	case tea.KeySpace:
		m.taskInput += " "
	case tea.KeyEnter:
		path, e := createWorkflowFile(m.boardDir, strings.TrimSpace(m.taskInput))
		if e != nil {
			m.taskNotice = e.Error()
			return m, nil
		}
		m.taskForm = ""
		return m, editWorkflowFile(m.tmuxPane, path)
	}
	return m, nil
}
