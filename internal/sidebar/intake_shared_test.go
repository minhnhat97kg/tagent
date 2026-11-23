package sidebar

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"tagent/internal/intake"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"testing"
)

func TestChatMentionsAndMultiline(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	fake := tmux.NewFake()
	restore := tmux.SetRunner(fake)
	defer restore()
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	m := Model{width: 44, height: 22, taskForm: "intake", notesScope: scope, boardDir: dir, intakeState: &intakeState{Token: 1, Scope: scope, Dir: dir}}
	m.sessions = []tmux.SessionInfo{{Windows: []tmux.WindowInfo{{Panes: []tmux.PaneInfo{{Name: "builder", PaneID: "%1", Path: dir, PanePID: 42}}}}}}
	defer intake.SetLiveSessions(func() []tmux.SessionInfo { return m.sessions })()
	m.taskInput = "@bu"
	next, _ := m.handleIntake(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	if m.taskInput != "@builder " {
		t.Fatal(m.taskInput)
	}
	next, _ = m.handleIntake(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	m = next.(Model)
	next, _ = m.handleIntake(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m = next.(Model)
	next, _ = m.handleIntake(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("world")})
	m = next.(Model)
	next, cmd := m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil || m.intakeState.Busy || len(m.intakeState.Shared.Response.Calls) != 1 {
		t.Fatal("mention did not become an immediate proposal")
	}
	call := m.intakeState.Shared.Response.Calls[0]
	if call.Args["prompt"] != "hello\nworld" || call.Args["pane"] != "%1" {
		t.Fatal(call)
	}
	if !strings.Contains(strings.Join(m.intakeLines(40, 20), "\n"), "approve") {
		t.Fatal("missing approval action")
	}
	next, _ = m.handleIntake(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	if m.intakeState.Shared.Status != "idle" {
		t.Fatal("cancel failed")
	}
	m.taskInput = "@builder approved send"
	next, _ = m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	old := executeIntakeTool
	defer func() { executeIntakeTool = old }()
	calls := 0
	executeIntakeTool = func(_ context.Context, gotDir, binary string, args []string) (string, error) {
		calls++
		if gotDir != dir || binary != "tagent" || strings.Join(args, "|") != "send|%1|approved send" {
			t.Fatal(gotDir, binary, args)
		}
		return "queued", nil
	}
	next, cmd = m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || calls != 0 {
		t.Fatal("approval did not claim work")
	}
	next, _ = m.intakeReply(cmd().(intakeReply))
	m = next.(Model)
	if calls != 1 || m.intakeState.Busy || m.intakeState.Shared.Status != "idle" {
		t.Fatal("approved tool did not complete")
	}
}

func TestClearChatIgnoresOldReply(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	shared, err := intake.ReserveSubmission("clear", "old request", "")
	if err != nil {
		t.Fatal(err)
	}
	m := Model{taskForm: "intake", intakeState: &intakeState{Token: 1, Scope: "clear", Shared: shared, Busy: true}}
	next, _ := m.handleIntake(tea.KeyMsg{Type: tea.KeyCtrlN})
	m = next.(Model)
	if m.intakeState.Busy || len(m.intakeState.Shared.Messages) != 0 {
		t.Fatal("clear did not reset chat")
	}
	next, _ = m.intakeReply(intakeReply{Token: 1, Err: intake.ErrStale})
	m = next.(Model)
	if m.taskNotice != "" || m.intakeState.Busy {
		t.Fatal("old reply changed new chat")
	}
	m.taskInput = "/clear"
	next, _ = m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.taskInput != "" {
		t.Fatal("slash clear did not clear composer")
	}
}

func TestSharedChatPanelsAndReopen(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir := t.TempDir()
	fakeChat(t, "ask_question", `{"text":"Which size?"}`)
	panel := func(token int64) Model {
		return Model{taskForm: "intake", notesScope: "shared", boardDir: dir, intakeState: &intakeState{Token: token, Scope: "shared", Dir: dir, Command: "stub"}}
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
	shared, e = intake.CompleteTypedSubmission("other", shared.Generation, intake.Result{Calls: []intake.ToolCall{{Name: "kill_agent", Args: map[string]string{"pane": "%1"}}}}, "kill", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	a = panel(4)
	a.notesScope = "other"
	a.intakeState.Scope = "other"
	a.reloadIntake()
	b = a
	old := executeIntakeTool
	defer func() { executeIntakeTool = old }()
	calls := 0
	executeIntakeTool = func(context.Context, string, string, []string) (string, error) { calls++; return "done", nil }
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
	if e != nil || len(original.Response.Calls) != 0 {
		t.Fatal("scope leak")
	}
}
