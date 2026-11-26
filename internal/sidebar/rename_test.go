package sidebar

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tmux"
)

func TestRenameKey(t *testing.T) {
	if _, ok := tmux.RunTmux("new-session", "-d", "-s", "rename", "sleep 60"); !ok {
		t.Skip("no tmux")
	}
	defer tmux.RunTmux("kill-session", "-t", "rename")
	out, _ := tmux.RunTmux("list-panes", "-t", "rename", "-F", "#{pane_id}")
	id := strings.TrimSpace(out)
	tmux.SetPaneOption(id, tmux.PaneAgent, "claude")
	m := Model{width: 40, height: 20, sessions: tmux.QuerySessions()}
	old := m.sessions[0].Windows[0].Panes[0].Name
	m.rows = []Row{{PaneID: id}}

	m, _, _ = m.agentsKey("R")
	if m.taskForm != "rename" || m.taskInput != old || !strings.Contains(ansi.Strip(m.taskFormView()), "Rename "+old) {
		t.Fatalf("R did not open the form: %q %q", m.taskForm, m.taskInput)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Rename "+old) {
		t.Fatal("the narrow sidebar does not show the form")
	}
	for _, key := range []tea.KeyMsg{{Type: tea.KeyCtrlU}, {Type: tea.KeyRunes, Runes: []rune("boss")}} {
		next, _ := m.handleTaskForm(key)
		m = next.(Model)
	}
	_, cmd := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEnter})
	cmd()
	if got := tmux.GetPaneOptionValue(id, tmux.PaneName); got != "boss" {
		t.Fatalf("name = %q", got)
	}
}
