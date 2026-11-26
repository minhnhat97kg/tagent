package main

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"tagent/internal/board"
	"tagent/internal/tmux"
	"tagent/internal/web"
)

func TestSecondWebAddsWorkspaceToRunning(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_AGENT_WEB_TOKEN", "tok")
	lock, _, err := web.Claim()
	if err != nil || lock == nil {
		t.Fatalf("claim: %v", err)
	}
	go lock.Serve("127.0.0.1:0", "tok", t.TempDir())
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if _, in, _ := web.Claim(); in.Addr != "" || time.Now().After(deadline) {
			break
		}
	}
	other := t.TempDir()
	t.Chdir(other)
	var out, errOut bytes.Buffer
	if code := cmdWeb([]string{"--addr", "127.0.0.1:1"}, &out, &errOut); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("added ")) {
		t.Fatalf("stdout: %q", out.String())
	}
	if !slices.Contains(board.Workspaces(), other) {
		t.Fatalf("%s not a workspace: %v", other, board.Workspaces())
	}
}

func TestBareTagentSkipsAutoSidebarPanes(t *testing.T) {
	if cwdIsWorkspace("sidebar") || !cwdIsWorkspace("") {
		t.Fatal("only panes that are not auto sidebars add their cwd")
	}
}

func TestBackgroundWebTakesOverWhenLockFrees(t *testing.T) {
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	holder, _, err := web.Claim()
	if err != nil || holder == nil {
		t.Fatalf("holder claim: %v", err)
	}
	published := func() bool {
		for _, c := range fake.Calls() {
			if slices.Contains(c, "@agent_web_status") {
				return true
			}
		}
		return false
	}
	go serveBackground("127.0.0.1:0", "mine")
	time.Sleep(1500 * time.Millisecond)
	if published() {
		t.Fatal("background web reported itself while another instance held the lock")
	}
	holder.Release()
	for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline) && !published(); time.Sleep(50 * time.Millisecond) {
	}
	if !published() {
		t.Fatal("background web did not take over the freed lock")
	}
}
