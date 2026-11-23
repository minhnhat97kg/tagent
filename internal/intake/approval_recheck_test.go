package intake

import (
	"context"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"testing"
)

// An agent that changes identity between proposal and approval must not receive the prompt.
func TestApprovalRechecksTarget(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	defer func() { liveSessions = tmux.QuerySessions }()
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	pane := func(pid int) []tmux.SessionInfo {
		return []tmux.SessionInfo{{Windows: []tmux.WindowInfo{{Panes: []tmux.PaneInfo{{PaneID: "%3", Path: dir, PanePID: pid, StartedAt: 1, Agent: "claude", Status: "idle"}}}}}}
	}
	liveSessions = func() []tmux.SessionInfo { return pane(7) }
	call := ToolCall{Name: "send_prompt", Args: map[string]string{"pane": "%3", "prompt": "go"}}
	if err := freezeCall(dir, &call); err != nil {
		t.Fatal(err)
	}
	s, err := ReserveSubmission(scope, "send", "")
	if err != nil {
		t.Fatal(err)
	}
	proposed, err := CompleteTypedSubmission(scope, s.Generation, Result{Calls: []ToolCall{call}}, "proposal", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := ClaimApproval(scope, proposed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	liveSessions = func() []tmux.SessionInfo { return pane(8) }
	ran := false
	if _, err = ExecuteClaimedTools(scope, dir, claimed, func(context.Context, string, string, []string) (string, error) {
		ran = true
		return "", nil
	}); err != nil {
		t.Fatal(err)
	}
	final, _ := LoadConversation(scope)
	if ran || !hasToolError(final, "agent changed") {
		t.Fatal("changed target was executed", ran, final.Messages)
	}
}

func hasToolError(c Conversation, text string) bool {
	for _, m := range c.Messages {
		if m.Tool != nil && strings.Contains(m.Tool.Output, text) {
			return true
		}
	}
	return false
}
