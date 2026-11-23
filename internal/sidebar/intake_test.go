package sidebar

import (
	"tagent/internal/tasks"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIntakePopup(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	fakeChat(t, "ask_question", `{"text":"Scope?"}`)
	s := &intakeState{Token: time.Now().UnixNano(), Scope: scope, Dir: dir, Command: "stub"}
	m := Model{notesScope: scope, boardDir: dir, taskForm: "intake", intakeState: s, taskInput: "idea"}
	model, cmd := m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if cmd == nil || !m.intakeState.Busy {
		t.Fatal("not thinking")
	}
	model, _ = m.intakeReply(cmd().(intakeReply))
	m = model.(Model)
	if m.intakeState.Question != "Scope?" {
		t.Fatal(m.intakeState)
	}
	fakeChat(t, "propose_ticket", `{"title":"Build intake","goal":"Clarify","acceptance":["Works"],"outOfScope":["Other"]}`)
	m.taskInput = "small"
	model, cmd = m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	model, _ = m.intakeReply(cmd().(intakeReply))
	m = model.(Model)
	if m.intakeState.Draft.Title == "" {
		t.Fatal("no draft")
	}
	model, cmd = m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if cmd == nil {
		t.Fatal("no save")
	}
	cmd()
	if len(tasks.List(scope)) != 1 {
		t.Fatal("not created")
	}
	model, _ = m.intakeReply(intakeReply{Token: s.Token})
	if model.(Model).taskForm != "" {
		t.Fatal("stale reopened")
	}
}
