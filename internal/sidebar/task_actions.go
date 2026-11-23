package sidebar

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"tagent/internal/board"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var newAgentChoices = []string{"claude", "codex", "opencode"}

type taskActionMsg struct{ Text string }

func actionResult(text string, err error) taskActionMsg {
	if err != nil {
		return taskActionMsg{"! " + err.Error()}
	}
	return taskActionMsg{"✓ " + text}
}
func (m Model) handleTaskForm(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.taskForm == "attachments" {
		list := m.attachments
		switch key.String() {
		case "esc":
			m.taskForm = ""
		case "j", "down":
			m.taskStep = min(max(0, len(list)-1), m.taskStep+1)
		case "k", "up":
			m.taskStep = max(0, m.taskStep-1)
		case "enter":
			if len(list) > 0 {
				m.taskForm = ""
				return m, openTaskAttachment(m.notesScope, m.boardDir, m.notes[m.taskCursor].ID, list[m.taskStep].Ref, m.tmuxPane)
			}
		}
		return m, nil
	}
	if m.taskForm == "workflow-new" {
		return m.handleWorkflowName(key)
	}
	if m.taskForm == "agent-model" || m.taskForm == "agent-new" {
		return m.handleAgentForm(key)
	}
	if m.taskForm == "new-agent" {
		switch key.String() {
		case "esc":
			m.taskForm = ""
		case "j", "down":
			m.taskStep = min(len(newAgentChoices)-1, m.taskStep+1)
		case "k", "up":
			m.taskStep = max(0, m.taskStep-1)
		case "enter":
			choice := newAgentChoices[m.taskStep]
			m.taskForm = ""
			return m, func() tea.Msg { return spawnChosenAgent(choice) }
		}
		return m, nil
	}
	if key.String() == "esc" {
		m.taskForm = ""
		return m, nil
	}
	if m.taskForm == "default" {
		switch key.String() {
		case "j", "down":
			m.taskStep = min(len(m.boardWorkflows), m.taskStep+1)
		case "k", "up":
			m.taskStep = max(0, m.taskStep-1)
		case "enter":
			name := ""
			if m.taskStep < len(m.boardWorkflows) {
				name = m.boardWorkflows[m.taskStep].Name
			}
			dir := m.boardDir
			m.taskForm = ""
			return m, func() tea.Msg {
				err := tasks.SetDefaultWorkflow(dir, name)
				return reloadBoardConfig(dir, err)
			}
		}
		return m, nil
	}
	if m.taskForm == "start" {
		switch key.String() {
		case "j", "down":
			m.taskStep = min(len(m.boardWorkflows), m.taskStep+1)
		case "k", "up":
			m.taskStep = max(0, m.taskStep-1)
		case "enter":
			name := "" // the board's default_workflow
			if m.taskStep < len(m.boardWorkflows) {
				name = m.boardWorkflows[m.taskStep].Name
			}
			dir, id := m.boardDir, m.notes[m.taskCursor].ID
			m.taskForm = ""
			return m, func() tea.Msg { return actionResult("Task started", board.Start(dir, id, name)) }
		}
		return m, nil
	}
	if m.taskForm == "chat" && key.Type == tea.KeyCtrlS {
		return m.saveChatInput()
	}
	switch key.Type {
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
		mode, input := m.taskForm, strings.TrimSpace(m.taskInput)
		m.taskForm = ""
		if mode == "chat" {
			pane := m.chat.Pane
			if input == "" || pane == "" {
				return m, nil
			}
			return m, sendTicketChat(pane, input)
		}
		if mode == "attach" {
			scope, id := m.notesScope, m.notes[m.taskCursor].ID
			return m, func() tea.Msg {
				return actionResult("File attached", tasks.Comment(scope, id, "user", "", []string{input}))
			}
		}
		if mode == "c" {
			root := m.scopeDir()
			if input == "" || root == "" {
				return m, nil
			}
			return m, gitDo(root, "Committed", "commit", "-m", input)
		}
		if mode == "i" {
			pane := m.promptPane
			if input == "" {
				return m, nil
			}
			return m, func() tea.Msg {
				tmux.TypeLine(pane, input)
				return actionResult("Prompt sent", nil)
			}
		}

	}
	return m, nil
}
func (m Model) taskFormView() string {
	width := max(10, m.width-4)
	title, hint := "Task detail", "Esc close"
	body := ""
	switch m.taskForm {
	case "attach":
		title, hint = "Attach file", "Local file path · Enter attach · Esc cancel"
	case "attachments":
		title, hint = "Attachments", "j/k select · Enter view · Esc cancel"
		for i, a := range m.attachments {
			prefix := "  "
			if i == m.taskStep {
				prefix = "› "
			}
			body += fmt.Sprintf("%s%s · %d bytes · %s · %s %s\n", prefix, a.Name, a.Size, a.Author, a.Time.Format("2006-01-02 15:04"), a.Error)
		}
	case "new-agent":
		title, hint = "New agent", "j/k select · Enter open · Esc cancel"
		for i, choice := range newAgentChoices {
			prefix := "  "
			if i == m.taskStep {
				prefix = "› "
			}
			body += prefix + choice + "\n"
		}
	case "default":
		title, hint = "Default workflow", m.keyHints("j/k select", "enter save", "esc cancel")
		for i, w := range m.boardWorkflows {
			prefix := "  "
			if i == m.taskStep {
				prefix = "› "
			}
			body += boardFit(prefix+w.Name+" · "+w.Description, width) + "\n"
		}
		prefix := "  "
		if m.taskStep == len(m.boardWorkflows) {
			prefix = "› "
		}
		body += prefix + "none (no default)"
	case "start":
		title, hint = "Start "+m.notes[m.taskCursor].Title, m.keyHints("j/k select", "enter start", "esc cancel")
		for i, w := range m.boardWorkflows {
			prefix := "  "
			if i == m.taskStep {
				prefix = "› "
			}
			body += boardFit(prefix+w.Name+" · "+w.Description, width) + "\n"
		}
		prefix := "  "
		if m.taskStep == len(m.boardWorkflows) {
			prefix = "› "
		}
		body += prefix + "default workflow"
		if m.boardConfig.DefaultWorkflow != "" {
			body += " (" + m.boardConfig.DefaultWorkflow + ")"
		}
	case "agent-model":
		title, hint = "Model for "+m.selectedAgentName(), "Enter save · Esc cancel · applies to new panes"
	case "agent-new":
		title, hint = "New agent", "Name · Enter create & edit · Esc cancel"
	case "workflow-new":
		title = "New workflow"
		hint = "Name · Enter create & edit · Esc cancel"
		body = m.taskNotice
	case "c":
		title, hint = "● Commit staged changes", "Message · Enter commit · Esc cancel"
	case "i":
		title, hint = "✎ Send prompt to "+m.promptPane, "Enter send · Esc cancel"
	case "chat":
		title, hint = "✎ Message the chat", "Enter send · ctrl+s save to ticket · Esc cancel"

	}
	if m.taskForm != "start" && m.taskForm != "default" && m.taskForm != "attachments" {
		body += "\n" + wrappedInput(m.taskInput, width, m.height)
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.Accent).Width(width).Render(lipgloss.NewStyle().Bold(true).Foreground(m.theme.Accent).Render(title) + "\n\n" + lipgloss.NewStyle().Width(width).Render(body) + "\n\n" + hint)
}

// scopePanes lists agent panes of this repo scope (each pane once), optionally of one harness.
func (m Model) scopePanes(harness string) []tmux.PaneInfo {
	var out []tmux.PaneInfo
	seen := map[string]bool{}
	for _, s := range m.sessions {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.Agent != "" && !seen[p.PaneID] && tasks.Scope(p.Path) == m.notesScope && (harness == "" || string(p.Agent) == harness) {
					seen[p.PaneID] = true
					out = append(out, p)
				}
			}
		}
	}
	return out
}

func agentLabel(p tmux.PaneInfo) string {
	if p.Name != "" {
		return p.Name
	}
	return string(p.Agent)
}

func (m Model) scopeDir() string {
	if ps := m.scopePanes(""); len(ps) > 0 {
		return ps[0].Path
	}
	return ""
}

func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

// Long text is edited outside the inline short-field form.
func editTaskText(scope string, task tasks.Task, pane string, comment bool) tea.Cmd {
	f, err := os.CreateTemp("", "tagent-task-*.md")
	if err != nil {
		return func() tea.Msg { return actionResult("", err) }
	}
	path := f.Name()
	prefill := ""
	if comment && task.Status == "blocked" {
		cfg, e := tasks.ConfigForScopeTask(scope, task)
		history, _ := tasks.ReadEvents(scope, 0)
		if e == nil {
			_, role, _ := tasks.BlockContext(task, cfg, history)
			if role != "" {
				prefill = "@" + role + " "
			}
		}
	}
	if !comment {
		_, err = f.WriteString(task.Text)
	} else {
		_, err = f.WriteString(prefill)
	}
	f.Close()
	if err != nil {
		os.Remove(path)
		return func() tea.Msg { return actionResult("", err) }
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	script := editor + " " + shellQuote(path)
	finish := func(err error) tea.Msg {
		defer os.Remove(path)
		if err == nil {
			var data []byte
			data, err = os.ReadFile(path)
			if err == nil {
				text := strings.TrimSpace(string(data))
				if comment {
					if text != "" && text != strings.TrimSpace(prefill) {
						err = tasks.Comment(scope, task.ID, "user", text, nil)
					}
				} else if task.ID == "" {
					if text != "" {
						_, err = tasks.Add(scope, text, nil)
					}
				} else {
					err = tasks.Edit(scope, task.ID, text)
				}
			}
		}
		return actionResult("Task updated", err)
	}
	if pane == "" {
		return tea.ExecProcess(exec.Command("sh", "-c", script), finish)
	}
	return func() tea.Msg {
		_, ok := tmux.RunTmux("display-popup", "-E", "-w", "90%", "-h", "90%", "-t", pane, script)
		if !ok {
			return finish(fmt.Errorf("editor popup failed"))
		}
		return finish(nil)
	}
}
