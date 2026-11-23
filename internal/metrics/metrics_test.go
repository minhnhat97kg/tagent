package metrics

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"tmux-agent/internal/tasks"
	"tmux-agent/internal/tmux"
)

func TestTaskUsageIntervalAndExclusiveSession(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	repo := t.TempDir()
	scope := tasks.Scope(repo)
	one, err := tasks.Add(scope, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	two, err := tasks.Add(scope, "second", nil)
	if err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "projects", "repo", "session-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript), 0700); err != nil {
		t.Fatal(err)
	}
	appendUsage := func(id string, in, out int) {
		f, err := os.OpenFile(transcript, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString(`{"type":"assistant","message":{"id":"` + id + `","usage":{"input_tokens":` + strconv.Itoa(in) + `,"output_tokens":` + strconv.Itoa(out) + `}}}` + "\n")
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	appendUsage("a", 10, 5)
	p := tmux.PaneInfo{Path: repo, Agent: tmux.AgentClaude, SessionID: "session-1", PaneID: "%1", Name: "agent", Status: tmux.StatusIdle}
	if err := Link(scope, one.ID, p); err != nil {
		t.Fatal(err)
	}
	if err := Link(scope, two.ID, p); err == nil {
		t.Fatal("second task shared active session")
	}
	appendUsage("b", 7, 3)
	state, err := Refresh(scope, []tmux.SessionInfo{{Windows: []tmux.WindowInfo{{Panes: []tmux.PaneInfo{p}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if n, missing := TaskTokens(state, one.ID); n != 10 || missing != 0 {
		t.Fatalf("task tokens=%d missing=%d, want 10/0", n, missing)
	}
	if err := End(scope, one.ID); err != nil {
		t.Fatal(err)
	}
	if err := Link(scope, two.ID, p); err != nil {
		t.Fatal(err)
	}
	state, err = Refresh(scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state.Sessions[Key(p)].Status != "closed" {
		t.Fatal("closed session missing from history")
	}
	if ActiveTask(state, Key(p)) != "" {
		t.Fatal("closed session kept active task")
	}
}
