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
	hints := m.keyHints("enter detail", "s start", "? help")
	if !strings.Contains(hints, "\x1b[") || !strings.Contains(ansi.Strip(hints), "s start") || ansi.StringWidth(hints) > 40 {
		t.Fatal(hints)
	}
}

// writeTestWorkflows writes two recipes; a-first sorts before sample.
func writeTestWorkflows(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a-first.yml": "name: a-first\njobs:\n  build: {run: echo ok}\n",
		"sample.yml":  "name: sample\ninputs:\n  goal: {required: true}\njobs:\n  build: {run: echo ok}\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(path, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStartWithStoredTypeSkipsPicker(t *testing.T) {
	dir := t.TempDir()
	writeTestWorkflows(t, dir)
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "ticket", nil, tasks.MutationOptions{Workflow: "sample"})
	if err != nil {
		t.Fatal(err)
	}
	m := boardTestModel()
	m.boardDir, m.notesScope, m.notes = dir, scope, []tasks.Task{ticket}
	m.boardWorkflows = workflow.List(dir)
	m, cmd := boardPress(m, "s")
	if m.taskForm != "" || cmd == nil {
		t.Fatalf("s opened a picker (%q) instead of starting the stored type", m.taskForm)
	}
	if msg := cmd().(taskActionMsg); strings.HasPrefix(msg.Text, "!") {
		t.Fatal(msg.Text)
	}
	got, _ := tasks.Get(scope, ticket.ID)
	if got.Status != "build" || got.Workflow != "sample" || got.Inputs["goal"] != "ticket" {
		t.Fatal(got)
	}
}

func TestTypeKeyPicksAndClearsWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeTestWorkflows(t, dir)
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "ticket", nil, tasks.MutationOptions{Workflow: "sample"})
	if err != nil {
		t.Fatal(err)
	}
	m := boardTestModel()
	m.boardDir, m.notesScope, m.notes = dir, scope, []tasks.Task{ticket}
	m.boardWorkflows = workflow.List(dir)
	m, _ = boardPress(m, "T")
	if m.taskForm != "type" || m.taskStep != 1 {
		t.Fatalf("T: form %q row %d, want the stored sample (1)", m.taskForm, m.taskStep)
	}
	for range m.boardWorkflows {
		m, _ = boardPress(m, "j")
	}
	next, cmd := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(Model).taskForm != "" || cmd == nil {
		t.Fatal("type picker did not submit")
	}
	if msg := cmd().(taskActionMsg); strings.HasPrefix(msg.Text, "!") {
		t.Fatal(msg.Text)
	}
	got, _ := tasks.Get(scope, ticket.ID)
	if got.Status != "todo" || got.Workflow != "" || got.Inputs != nil {
		t.Fatal(got)
	}
}

func TestNewTicketFormAddsWithWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeTestWorkflows(t, dir)
	scope := tasks.Scope(dir)
	m := boardTestModel()
	m.boardDir, m.notesScope, m.boardWorkflows = dir, scope, workflow.List(dir)
	m, _ = boardPress(m, "n")
	if m.taskForm != "new-task" || m.taskStep != len(m.boardWorkflows) {
		t.Fatalf("form %q opened on row %d, want none", m.taskForm, m.taskStep)
	}
	next, _ := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyUp}) // up to sample
	m = next.(Model)
	if m.taskStep != 1 {
		t.Fatalf("up moved to row %d, want sample (1)", m.taskStep)
	}
	// Enter hands the picked type to the editor; saving the text is what makes the ticket.
	if next, cmd := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEnter}); next.(Model).taskForm != "" || cmd == nil {
		t.Fatal("type picker did not open the editor")
	}
	if err := saveTaskText(scope, tasks.Task{}, "", false, "sample", "write the docs"); err != nil {
		t.Fatal(err)
	}
	all := tasks.List(scope)
	if len(all) != 1 || all[0].Workflow != "sample" || all[0].Inputs["goal"] != "write the docs" || all[0].Status != "todo" {
		t.Fatalf("added: %+v", all)
	}
}
