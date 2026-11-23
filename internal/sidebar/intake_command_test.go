package sidebar

import (
	"strings"
	"tagent/internal/intake"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIntakeAnswerCompletion(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	m := Model{taskForm: "intake", intakeState: &intakeState{Token: 1, Idea: "status?", Turns: []intake.Turn{{Question: "which?", Answer: "all"}}}}
	model, cmd := m.intakeReply(intakeReply{Token: 1, Raw: "ANSWER: Ready\nCMD: not executable"})
	m = model.(Model)
	if cmd != nil || m.intakeState.Answer == "" || m.intakeState.Draft.Title != "" || m.intakeState.ApprovedCommand != "" {
		t.Fatal("answer became action")
	}
	for _, size := range []int{0, 1, 10} {
		if len(m.intakeLines(12, size)) > size {
			t.Fatal("overflow")
		}
	}
	model, cmd = m.handleIntake(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = model.(Model)
	if cmd != nil || m.taskInput != "e" {
		t.Fatal("answer opened editor")
	}
	m.taskInput = "next"
	model, cmd = m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if cmd == nil || m.intakeState.Answer != "" || len(m.intakeState.Turns) != 0 || m.intakeState.Idea != "next" || m.intakeState.Token == 1 {
		t.Fatal("not fresh")
	}
	model, _ = m.intakeReply(intakeReply{Token: 1, Raw: "ANSWER: stale"})
	if model.(Model).intakeState.Answer != "" {
		t.Fatal("stale answer")
	}
	model, _ = m.handleIntake(tea.KeyMsg{Type: tea.KeyEsc})
	if model.(Model).intakeState != nil {
		t.Fatal("not closed")
	}
}

func TestIntakeQuickCommand(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	old := executeIntakeCommand
	defer func() { executeIntakeCommand = old }()
	calls := 0
	executeIntakeCommand = func(c, d string) (string, error) {
		calls++
		if c != "tagent list" {
			t.Fatal(c)
		}
		return "agents output", nil
	}
	m := Model{taskForm: "intake", taskInput: "reload", intakeState: &intakeState{Token: 1, Command: "echo 'CMD: tagent reload'"}}
	model, cmd := m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	model, _ = m.intakeReply(cmd().(intakeReply))
	m = model.(Model)
	if m.intakeState.ApprovedCommand == "" || calls != 0 {
		t.Fatal("ran without approval")
	}
	model, _ = m.handleIntake(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m = model.(Model)
	m.taskInput = "tagent install"
	model, _ = m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if !m.intakeState.EditingCommand {
		t.Fatal("unsafe edit accepted")
	}
	m.taskInput = "tagent list"
	model, _ = m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	if calls != 0 {
		t.Fatal("edit ran")
	}
	model, cmd = m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(Model)
	model, _ = m.Update(cmd())
	m = model.(Model)
	if calls != 1 || !strings.Contains(m.intakeState.Output, "agents output") {
		t.Fatal("no output")
	}
}
func TestIntakeEscalateAndReject(t *testing.T) {
	m := Model{taskForm: "intake", intakeState: &intakeState{Token: 1, Idea: "design a system"}}
	model, cmd := m.intakeReply(intakeReply{Token: 1, Raw: "ESCALATE: Requires judgement"})
	m = model.(Model)
	if m.intakeState.Escalate == "" || cmd == nil {
		t.Fatal("no escalation or clipboard")
	}
	m = Model{taskForm: "intake", intakeState: &intakeState{Token: 1}}
	model, _ = m.intakeReply(intakeReply{Token: 1, Raw: "CMD: tagent reload; evil"})
	if model.(Model).intakeState.ApprovedCommand != "" {
		t.Fatal("unsafe command")
	}
}
