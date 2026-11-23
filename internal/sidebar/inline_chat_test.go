package sidebar

import (
	"net/http"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestInlineAnswerResubmit(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	fakeChat(t, "answer", `{"text":"fresh"}`)
	m := Model{dashboard: true, agentFocus: 3, inlineIntake: true, notesScope: "repo", boardDir: t.TempDir(), taskInput: "modal"}
	m.intakeState = &intakeState{Token: 1, Scope: m.notesScope, Dir: m.boardDir, Idea: "status?", Answer: "Ready", Command: "stub"}
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
	next, _ = m.intakeReply(intakeReply{Token: m.intakeState.Token})
	if next.(Model).intakeState.Answer == "leaked" {
		t.Fatal("scope leaked")
	}
}

func TestInlineChatErrorThenResubmit(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	fakeChatHandler(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "LLM unavailable", 503) })
	m := Model{dashboard: true, agentFocus: 3, inlineIntake: true, notesScope: "repo", boardDir: t.TempDir(), taskInput: "build a dashboard"}
	m.intakeState = &intakeState{Token: 1, Scope: m.notesScope, Dir: m.boardDir, Command: "stub", Input: "build a dashboard"}
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("request not submitted")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.intakeState.Busy || m.intakeState.Shared.Status != "failed" {
		t.Fatal("failed request not recorded", m.intakeState.Shared.Status)
	}
	fakeChat(t, "ask_question", `{"text":"Details?"}`)
	m.intakeState.Input = "build a dashboard"
	next, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("retry not submitted")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.intakeState.Question != "Details?" || m.intakeState.Busy {
		t.Fatal("retry did not complete inline", m.intakeState)
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
	m.notesScope = "other"
	next, _ = m.intakeReply(intakeReply{Token: 1})
	if next.(Model).intakeState == nil || next.(Model).intakeState.Shared.Generation != 0 {
		t.Fatal("reload changed another scope")
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

func TestInlineChatCancel(t *testing.T) {
	m := Model{dashboard: true, agentFocus: 3, inlineIntake: true, notesScope: "repo", boardDir: "/repo", intakeState: &intakeState{Token: 1, Scope: "repo", Dir: "/repo"}}
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	next, _ = m.intakeReply(intakeReply{Token: 1})
	m = next.(Model)
	if m.intakeState != nil || m.agentFocus == 3 {
		t.Fatal("cancel did not invalidate")
	}
}

func TestChatBoundedStates(t *testing.T) {
	for _, size := range [][2]int{{1, 0}, {4, 1}, {25, 6}, {100, 30}} {
		for _, s := range []*intakeState{{}, {Busy: true}, {Question: "question\nmore"}, {Output: strings.Repeat("output\n", 40)}, {Raw: "invalid"}} {
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
