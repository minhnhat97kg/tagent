package sidebar

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestInlineAnswerResubmit(t *testing.T) {
	legacyIntakeFixture(t)
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	m := Model{dashboard: true, agentFocus: 3, inlineIntake: true, notesScope: "repo", boardDir: t.TempDir(), taskInput: "modal"}
	m.intakeState = &intakeState{Token: 1, Scope: m.notesScope, Dir: m.boardDir, Idea: "status?", Answer: "Ready", Command: "printf 'ANSWER: fresh'"}
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("next")})
	m = next.(Model)
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.taskInput != "modal" || m.intakeState.Input != "" || m.intakeState.Idea != "next" {
		t.Fatal("input leaked", m.intakeState)
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.intakeState.Answer != "fresh" || m.taskInput != "modal" {
		t.Fatal("answer not displayed")
	}
	m.notesScope = "other"
	next, _ = m.intakeReply(intakeReply{Token: m.intakeState.Token, Raw: "ANSWER: leaked"})
	if next.(Model).intakeState.Answer == "leaked" {
		t.Fatal("scope leaked")
	}
}

func TestInlineChatErrorThenResubmit(t *testing.T) {
	legacyIntakeFixture(t)
	for _, followup := range []bool{false, true} {
		t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
		m := Model{dashboard: true, agentFocus: 3, inlineIntake: true, notesScope: "repo", boardDir: t.TempDir(), taskInput: "unrelated form"}
		m.intakeState = &intakeState{Token: 1, Scope: m.notesScope, Dir: m.boardDir, Command: "printf 'Q: Details?'", Input: "build a dashboard"}
		next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
		if cmd == nil {
			t.Fatal("initial request not submitted")
		}
		if followup {
			next, _ = m.Update(cmd())
			m = next.(Model)
			next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("small")})
			m = next.(Model)
			next, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(Model)
			if cmd == nil || len(m.intakeState.Turns) != 1 {
				t.Fatal("followup not submitted")
			}
		}
		next, _ = m.Update(intakeReply{Token: 1, Err: errors.New("LLM unavailable")})
		m = next.(Model)
		if m.taskForm != "" || m.intakeState.Busy || m.intakeState.Input != "build a dashboard" || m.taskNotice != "LLM unavailable" {
			t.Fatal("failed request not restored inline", m.intakeState)
		}
		if m.intakeState.Token == 1 || m.intakeState.Idea != "" || len(m.intakeState.Turns) != 0 {
			t.Fatal("failed conversation not reset")
		}
		next, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
		if cmd == nil || !m.intakeState.Busy {
			t.Fatal("Enter did not retry restored request")
		}
		// A late result from the failed conversation cannot replace the retry.
		next, _ = m.Update(intakeReply{Token: 1, Raw: "Q: stale"})
		m = next.(Model)
		if !m.intakeState.Busy {
			t.Fatal("accepted stale reply")
		}
		next, _ = m.Update(cmd())
		m = next.(Model)
		if m.intakeState.Question != "Details?" || m.intakeState.Busy || m.taskInput != "unrelated form" {
			t.Fatal("retry did not complete inline", m.intakeState)
		}
	}
}

func TestInlineChatRouting(t *testing.T) {
	m := Model{dashboard: true, agentFocus: 3, inlineIntake: true, notesScope: "repo", boardDir: "/repo", theme: DefaultTheme(), intakeState: &intakeState{Token: 1, Scope: "repo", Dir: "/repo"}}
	for _, text := range []string{"j", "k", "q", "?", "["} {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
		m = next.(Model)
	}
	if m.intakeState.Input != "jkq?[" || m.helpOpen {
		t.Fatal("chat shortcuts intercepted input", m.intakeState)
	}
	m.intakeState.Busy = true
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.intakeReply(intakeReply{Token: 1, Raw: "Q: Details?"})
	m = next.(Model)
	if m.intakeState.Question != "Details?" {
		t.Fatal("lost background reply")
	}
	m.notesScope = "other"
	next, _ = m.intakeReply(intakeReply{Token: 1, Raw: "Q: Stale?"})
	if next.(Model).intakeState.Question == "Stale?" {
		t.Fatal("accepted stale scope")
	}
}

func TestInlineChatFocusAndMouse(t *testing.T) {
	for _, width := range []int{60, 140} {
		m := Model{dashboard: true, width: width, height: 55, theme: DefaultTheme()}
		for want := 1; want <= 4; want++ {
			next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
			m = next.(Model)
			if m.agentFocus != want%4 {
				t.Fatalf("focus=%d", m.agentFocus)
			}
			if m.intakeState != nil {
				t.Fatal("Tab initialized conversation")
			}
		}
		rows, _ := m.companionPanel(max(6, width-1))
		g := m.dashGeometry(rows)
		x, y := g.left, g.top+g.second
		if width < 90 {
			x, y = 0, g.top+g.first+g.second
		}
		next, _ := m.handleMouse(tea.MouseMsg{X: x + 2, Y: y + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		m = next.(Model)
		if m.agentFocus != 3 {
			t.Fatal("Chat click did not focus")
		}
		next, _ = m.handleMouse(tea.MouseMsg{X: x + 2, Y: y + 2, Button: tea.MouseButtonWheelDown})
		if next.(Model).cursor != m.cursor || next.(Model).taskCursor != m.taskCursor {
			t.Fatal("Chat wheel navigated other panels")
		}
	}
}

func TestInlineChatCancelAndApproval(t *testing.T) {
	old := executeIntakeCommand
	defer func() { executeIntakeCommand = old }()
	calls := 0
	executeIntakeCommand = func(command, dir string) (string, error) { calls++; return "done", nil }
	m := Model{dashboard: true, agentFocus: 3, inlineIntake: true, notesScope: "repo", boardDir: "/repo", taskInput: "form input", intakeState: &intakeState{Token: 1, Scope: "repo", Dir: "/repo"}}
	next, _ := m.intakeReply(intakeReply{Token: 1, Raw: "CMD: tagent list"})
	m = next.(Model)
	if calls != 0 || m.taskForm != "" {
		t.Fatal("approval not inline")
	}
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("no command after approval")
	}
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if calls != 1 || m.intakeState.Output != "done" || m.taskInput != "form input" {
		t.Fatal("lost output or corrupted form input")
	}
	m.agentFocus = 3
	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	next, _ = m.intakeReply(intakeReply{Token: 1, Raw: "Q: stale"})
	m = next.(Model)
	if m.intakeState != nil || m.agentFocus == 3 {
		t.Fatal("cancel did not invalidate")
	}
}

func TestChatBoundedStates(t *testing.T) {
	for _, size := range [][2]int{{1, 0}, {4, 1}, {25, 6}, {100, 30}} {
		for _, s := range []*intakeState{{}, {Busy: true}, {Question: "question\nmore"}, {ApprovedCommand: "echo hello"}, {Output: strings.Repeat("output\n", 40)}, {Raw: "invalid"}} {
			m := Model{intakeState: s, taskInput: "multi\nline", taskNotice: "error", theme: DefaultTheme()}
			lines := m.intakeLines(size[0], size[1])
			if len(lines) > size[1] {
				t.Fatal("height overflow")
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatal("width overflow")
				}
			}
		}
	}
}
