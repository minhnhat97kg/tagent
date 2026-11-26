package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"tagent/internal/board"
	"tagent/internal/roles"
	"tagent/internal/tmux"
	"testing"
	"time"
)

// setupSignTest starts a tmux server whose first pane is an agent pane named
// "fe" (idle), and returns that pane's id plus the file the fake agent in the
// planner pane writes every line it receives to.
func setupSignTest(t *testing.T) (fePane, received string) {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux unavailable")
	}
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	fake := filepath.Join(dir, "claude")
	os.WriteFile(fake, []byte("#!/bin/sh\ntmux set-option -p -t \"$TMUX_PANE\" @pane_agent claude\ntmux set-option -p -t \"$TMUX_PANE\" @pane_status idle\nwhile IFS= read -r line; do printf '%s\\n' \"$line\" >> \"$SIGN_TEST_RECEIVED\"; done\n"), 0700)
	received = filepath.Join(dir, "received")
	t.Setenv("SIGN_TEST_RECEIVED", received)
	if b, err := exec.Command("tmux", "-f", "/dev/null", "new-session", "-d", "-c", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(b))
	}
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	tmux.RunTmux("set-option", "-g", "@agent_scrape", "off")
	os.MkdirAll(roles.GlobalDir(), 0700)
	os.WriteFile(filepath.Join(roles.GlobalDir(), "planner.md"), []byte("Plan carefully."), 0600)
	os.WriteFile(filepath.Join(roles.GlobalDir(), "profiles.toml"), []byte("[planner]\nharness = '"+fake+"'\n"), 0600)
	fePane, _ = tmux.RunTmuxCapture("display-message", "-p", "#{pane_id}")
	fePane = strings.TrimSpace(fePane)
	tmux.SetPaneOption(fePane, tmux.PaneName, "fe")
	tmux.SetPaneOption(fePane, tmux.PaneStatusOpt, string(tmux.StatusIdle))
	return fePane, received
}

func waitReceived(t *testing.T, file, want string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		b, _ := os.ReadFile(file)
		if strings.Contains(string(b), want) {
			return string(b)
		}
		if time.Now().After(deadline) {
			t.Fatalf("delivery timeout waiting for %q, got: %s", want, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The board dispatcher is a system send, not an agent message: even when the
// process runs with TMUX_PANE pointing at an agent pane (the scraper inherits
// it from the pane that started it), the prompt must not be signed as from
// that agent.
func TestBoardDispatchIsNotSignedFromInheritedPane(t *testing.T) {
	fePane, received := setupSignTest(t)
	t.Setenv("TMUX_PANE", fePane)
	if err := dispatchBoardAction(board.Action{Dir: t.TempDir(), Agent: "planner", Prompt: "dispatch prompt"}); err != nil {
		t.Fatal(err)
	}
	got := waitReceived(t, received, "dispatch prompt")
	if strings.Contains(got, "[from ") {
		t.Fatalf("board dispatch was signed as an agent message: %s", got)
	}
}

// Agent-to-agent `tagent send` must keep signing so the receiver knows whom to answer.
func TestAgentSendIsSignedFromAgentPane(t *testing.T) {
	fePane, received := setupSignTest(t)
	planner := tmux.ResolvePane("planner")
	if planner == "planner" {
		// Spawn the planner pane through the same path the dispatcher uses.
		if err := dispatchBoardAction(board.Action{Dir: t.TempDir(), Agent: "planner", Prompt: "warm up"}); err != nil {
			t.Fatal(err)
		}
		waitReceived(t, received, "warm up")
	}
	t.Setenv("TMUX_PANE", fePane)
	if code := cmdSend([]string{"planner", "hello planner"}, os.Stdout, os.Stderr); code != exitOK {
		t.Fatalf("cmdSend exit %d", code)
	}
	got := waitReceived(t, received, "hello planner")
	if !strings.Contains(got, "[from fe; reply: tagent send fe <text>] hello planner") {
		t.Fatalf("agent-to-agent send not signed: %s", got)
	}
}
