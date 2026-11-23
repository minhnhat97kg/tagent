package sidebar

import (
	"os"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWorkflowPanelFocus(t *testing.T) {
	m := Model{boardMode: true, width: 100, height: 40, boardWorkflows: []workflow.Workflow{{Name: "one", File: "one.yml"}, {Name: "two", File: "two.yml"}}}
	m, _, handled := m.boardKey("tab")
	if !handled || m.focus != cardWorkflows {
		t.Fatal("no workflow focus")
	}
	m, _, _ = m.boardKey("j")
	if m.workflowCursor != 1 {
		t.Fatal(m.workflowCursor)
	}
	m, _, _ = m.boardKey("tab")
	if m.focus == cardWorkflows {
		t.Fatal("no ticket focus")
	}
	m, _, _ = m.boardKey("tab")
	m, _, _ = m.boardKey("enter")
	if !m.boardFlow || m.flowWorkflow != "two.yml" {
		t.Fatal("wrong workflow view")
	}
}
func TestWorkflowAddAndReload(t *testing.T) {
	dir := t.TempDir()
	m := Model{boardMode: true, focus: cardWorkflows, boardDir: dir}
	m, _, _ = m.boardKey("a")
	if m.taskForm != "workflow-new" {
		t.Fatal("no name form")
	}
	m.taskInput = "example"
	model, cmd := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if cmd == nil || m.taskForm != "" {
		t.Fatal("no editor")
	}
	path := workflow.LocalDir(tasks.MainRoot(dir)) + "/example.yml"
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = createWorkflowFile(dir, "example"); e == nil {
		t.Fatal("overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("changed file")
	}
	if _, e = createWorkflowFile(dir, "../escape"); e == nil {
		t.Fatal("unsafe name")
	}
	edited := strings.Replace(string(before), "Describe when to use this workflow", "Edited description", 1)
	if e = os.WriteFile(path, []byte(edited), 0600); e != nil {
		t.Fatal(e)
	}
	model, _ = m.Update(workflowsEditedMsg{})
	m = model.(Model)
	if len(m.boardWorkflows) != 1 || m.boardWorkflows[0].Description != "Edited description" {
		t.Fatal("not reloaded")
	}
	_, cmd, _ = m.boardKey("e")
	if cmd == nil {
		t.Fatal("edit command missing")
	}
}
func TestWorkflowDeleteConfirmation(t *testing.T) {
	dir := t.TempDir()
	path, e := createWorkflowFile(dir, "example")
	if e != nil {
		t.Fatal(e)
	}
	m := Model{boardMode: true, focus: cardWorkflows, boardDir: dir, boardWorkflows: workflow.List(tasks.MainRoot(dir))}
	m, cmd, _ := m.boardKey("x")
	if cmd != nil || m.workflowDelete != path {
		t.Fatal("no confirmation")
	}
	m, _, _ = m.boardKey("n")
	if _, e = os.Stat(path); e != nil || m.workflowDelete != "" {
		t.Fatal("cancel failed")
	}
	m, _, _ = m.boardKey("x")
	m, cmd, _ = m.boardKey("y")
	if cmd == nil {
		t.Fatal("no delete")
	}
	model, _ := m.Update(cmd())
	m = model.(Model)
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("file not deleted")
	}
	if len(m.boardWorkflows) != 0 {
		t.Fatal("list not reloaded")
	}
}
