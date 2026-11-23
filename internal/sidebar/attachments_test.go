package sidebar

import (
	tea "github.com/charmbracelet/bubbletea"
	"os"
	"path/filepath"
	"strings"
	"tagent/internal/tasks"
	"testing"
)

func TestAttachmentSelectorAndAttach(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "Files", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := boardTestModel()
	m.boardDir = dir
	m.notesScope = scope
	m.notes = []tasks.Task{ticket}
	m.taskCursor = 0
	m.boardDrawer = true
	m.boardMode = true
	m, _, handled := m.boardKey("A")
	if !handled || m.taskForm != "attach" {
		t.Fatal("attach key")
	}
	p := filepath.Join(dir, "example.log")
	os.WriteFile(p, []byte("hello\n"), 0600)
	m.taskInput = p
	next, cmd := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("missing attach command")
	}
	result := cmd().(taskActionMsg)
	if strings.HasPrefix(result.Text, "!") {
		t.Fatal(result.Text)
	}
	m, _, handled = m.boardKey("a")
	if !handled || m.taskForm != "attachments" {
		t.Fatal("selector key")
	}
	if !strings.Contains(m.taskFormView(), "example.log") {
		t.Fatal(m.taskFormView())
	}
	next, _ = m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEsc})
	if next.(Model).taskForm != "" {
		t.Fatal("escape")
	}
	list, _ := tasks.AttachmentList(scope, dir, ticket.ID)
	if len(list) != 1 {
		t.Fatal(list)
	}
	if cmd = openTaskAttachment(scope, dir, ticket.ID, "invalid", ""); cmd == nil || !strings.HasPrefix(cmd().(taskActionMsg).Text, "!") {
		t.Fatal("unauthorized pager")
	}
}
