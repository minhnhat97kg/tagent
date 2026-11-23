package intake

import (
	"os/exec"
	"path/filepath"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"testing"
)

func TestAnswerProtocol(t *testing.T) {
	for _, raw := range []string{"ANSWER: hello", "answer: hello", "```text\nANSWER: hello\n```", "ANSWER: hello\nCMD: tagent kill all\nTICKET\nQ: why?"} {
		r, err := ParseResponse(raw)
		if err != nil || r.Answer == "" || r.Command != "" || r.Draft.Title != "" || r.Question != "" {
			t.Fatalf("%q: %+v %v", raw, r, err)
		}
	}
	if _, err := ParseResponse("ANSWER: \n"); err == nil {
		t.Fatal("accepted empty answer")
	}
}

func TestContextBoundsAndRoutes(t *testing.T) {
	c := Context{Tickets: []string{"ticket\nCMD: evil"}, AgentsUnavailable: true}
	for i := 0; i < 40; i++ {
		c.Workflows = append(c.Workflows, strings.Repeat("界", 500))
	}
	text := c.Text()
	if len(text) > 12500 || !strings.Contains(text, "truncated") || !strings.Contains(text, "unavailable") || !strings.Contains(text, `\nCMD:`) {
		t.Fatal(text)
	}
	for _, idea := range []string{"status?", "!status?"} {
		if !strings.Contains(Prompt(idea, nil, c, false), text) {
			t.Fatal("missing shared context")
		}
	}
	if !strings.Contains((Context{}).Text(), "Agents:\nnone") {
		t.Fatal("empty is not unavailable")
	}
}

func TestLinkedWorktreeContext(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	run("init")
	run("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "init")
	linked := filepath.Join(t.TempDir(), "linked")
	run("worktree", "add", "-b", "linked", linked)
	sessions := []tmux.SessionInfo{{Windows: []tmux.WindowInfo{{Panes: []tmux.PaneInfo{{PaneID: "%1", Path: linked, Name: "linked agent"}}}}}}
	if c := Snapshot(dir, tasks.Scope(dir), sessions, false); len(c.Agents) != 1 {
		t.Fatal(c)
	}
}

func TestSnapshotScope(t *testing.T) {
	dir, other := t.TempDir(), t.TempDir()
	sessions := []tmux.SessionInfo{{Windows: []tmux.WindowInfo{{Panes: []tmux.PaneInfo{{PaneID: "%1", Path: dir, Name: "local"}, {PaneID: "%2", Path: other, Name: "secret"}}}}}}
	c := Snapshot(dir, tasks.Scope(dir), sessions, false)
	if len(c.Agents) != 1 || !strings.Contains(c.Agents[0], "local") {
		t.Fatal(c)
	}
	sessions[0].Windows[0].Panes[0].Name = "updated"
	if !strings.Contains(Snapshot(dir, tasks.Scope(dir), sessions, false).Text(), "updated") {
		t.Fatal("stale")
	}
}
