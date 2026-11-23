package sidebar

import (
	tea "github.com/charmbracelet/bubbletea"
	"tagent/internal/intake"
	"testing"
)

func TestSharedChatPanelsAndReopen(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir := t.TempDir()
	panel := func(token int64) Model {
		return Model{taskForm: "intake", notesScope: "shared", boardDir: dir, intakeState: &intakeState{Token: token, Scope: "shared", Dir: dir, Command: "printf 'Q: Which size?'"}}
	}
	a, b := panel(1), panel(2)
	a.taskInput = "build a thing"
	next, cmd := a.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	a = next.(Model)
	if cmd == nil {
		t.Fatal("not submitted")
	}
	b.reloadIntake()
	if !b.intakeState.Busy || b.intakeState.Idea != "build a thing" {
		t.Fatal(b.intakeState)
	}
	// Closing a panel does not cancel its in-flight durable request.
	_, _ = a.handleIntake(tea.KeyMsg{Type: tea.KeyEsc})
	cmd()
	b.reloadIntake()
	if b.intakeState.Question != "Which size?" {
		t.Fatal(b.intakeState)
	}
	reopened := panel(3)
	reopened.reloadIntake()
	if reopened.intakeState.Question != "Which size?" {
		t.Fatal("lost clarification")
	}
	reopened.taskInput = "small"
	next, cmd = reopened.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	reopened = next.(Model)
	if cmd == nil {
		t.Fatal("not answered")
	}
	cmd()
	shared, e := intake.LoadConversation("shared")
	if e != nil || len(shared.Turns) != 1 || shared.Turns[0].Answer != "small" {
		t.Fatal(shared, e)
	}
	// A remote command proposal is observed and can only be claimed once.
	shared, e = intake.ReserveSubmission("other", "!reload", "")
	if e != nil {
		t.Fatal(e)
	}
	shared, e = intake.CompleteSubmission("other", shared.Generation, "CMD: tagent reload", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	a = panel(4)
	a.notesScope = "other"
	a.intakeState.Scope = "other"
	a.reloadIntake()
	b = a
	old := executeIntakeCommand
	defer func() { executeIntakeCommand = old }()
	calls := 0
	executeIntakeCommand = func(string, string) (string, error) { calls++; return "done", nil }
	_, first := a.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	_, second := b.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	if first == nil || second != nil {
		t.Fatal("competing approval accepted")
	}
	first()
	if calls != 1 {
		t.Fatal(calls)
	}
	original, e := intake.LoadConversation("shared")
	if e != nil || original.Response.Command != "" {
		t.Fatal("scope leak")
	}
}
