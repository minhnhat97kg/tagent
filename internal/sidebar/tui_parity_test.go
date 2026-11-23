package sidebar

import (
	"net/http"
	"net/http/httptest"
	"tagent/internal/intake"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestReceptionistChoice(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	m := Model{dashboard: true, taskForm: "new-agent", taskStep: len(newAgentChoices) - 1, notesScope: "repo", boardDir: t.TempDir()}
	next, cmd := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if cmd != nil || got.agentFocus != 3 || !got.inlineIntake || got.taskForm != "" || got.intakeState == nil {
		t.Fatalf("did not focus Chat: %+v", got.intakeState)
	}
}

func TestSidebarNativeIntakeRouting(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/chat" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":{"tool_calls":[{"function":{"name":"ask_question","arguments":{"text":"Which scope?"}}}]}}`))
	}))
	defer server.Close()
	old := runChat
	defer func() { runChat = old }()
	runChat = func(model string, s intake.Submission) {
		intake.RunReceptionist(s, intake.Harness{Endpoint: server.URL, Model: model})
	}
	dir := t.TempDir()
	m := Model{taskForm: "intake", notesScope: "repo", boardDir: dir, taskInput: "build a feature", intakeState: &intakeState{Token: 1, Scope: "repo", Dir: dir, Command: "test-model"}}
	next, cmd := m.handleIntake(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	next, _ = m.Update(cmd())
	m = next.(Model)
	if calls != 1 || m.intakeState.Question != "Which scope?" {
		t.Fatalf("native routing failed: %d %+v", calls, m.intakeState)
	}
}

func TestAvatarDefaultOff(t *testing.T) {
	c := avatarConfigFromOptions(nil)
	if c.enabled() || c.llmEnabled() {
		t.Fatal("default avatar may not call models")
	}
	c.Character = "off"
	c.LLM = "some-model"
	if c.llmEnabled() {
		t.Fatal("off avatar may not summarize")
	}
}
