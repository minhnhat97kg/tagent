package sidebar

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"os"
	"path/filepath"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
	"testing"
)

func TestStartPicker(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "sample.yml"), []byte("name: sample\njobs:\n  build: {run: echo ok}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "ticket", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := boardTestModel()
	m.boardDir, m.notesScope, m.notes = dir, scope, []tasks.Task{ticket}
	m.boardWorkflows = workflow.List(dir)
	m, _ = boardPress(m, "s")
	if m.taskForm != "start" {
		t.Fatal(m.taskForm)
	}
	next, cmd := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(Model).taskForm != "" || cmd == nil {
		t.Fatal("picker did not submit")
	}
	msg := cmd().(taskActionMsg)
	if strings.HasPrefix(msg.Text, "!") {
		t.Fatal(msg.Text)
	}
	got, _ := tasks.Get(scope, ticket.ID)
	if got.Status != "build" || got.Workflow != "sample" {
		t.Fatal(got)
	}
}

func TestWrappedPopupInput(t *testing.T) {
	orig := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(orig)
	input := strings.Repeat("x", 300)
	m := boardTestModel()
	m.width, m.height, m.taskInput = 40, 40, input
	m.taskForm = "i"
	if got := strings.Count(ansi.Strip(m.taskFormView()), "x"); got != 300 {
		t.Fatalf("task: %d", got)
	}
	m.taskForm = "intake"
	m.intakeState = &intakeState{}
	if got := strings.Count(ansi.Strip(m.intakeView()), "x"); got != 300 {
		t.Fatalf("intake: %d", got)
	}
	for _, line := range strings.Split(ansi.Strip(m.intakeView()), "\n") {
		if ansi.StringWidth(line) > 40 {
			t.Fatal(line)
		}
	}
	hints := m.keyHints("enter detail", "s start", "? help")
	if !strings.Contains(hints, "\x1b[") || !strings.Contains(ansi.Strip(hints), "s start") || ansi.StringWidth(hints) > 40 {
		t.Fatal(hints)
	}
}
