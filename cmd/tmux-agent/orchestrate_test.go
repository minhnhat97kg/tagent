package main

import (
	"testing"
	"tmux-agent/internal/tmux"
)

func TestPickIdle(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	sessions := []tmux.SessionInfo{{Windows: []tmux.WindowInfo{{Panes: []tmux.PaneInfo{
		{PaneID: "%1", Name: "busy", Status: tmux.StatusRunning, Path: "/r"},
		{PaneID: "%2", Name: "elsewhere", Status: tmux.StatusIdle, Path: "/other"},
		{PaneID: "%3", Name: "free", Status: tmux.StatusIdle, Path: "/r/sub"},
	}}}}}
	if p, n := pickIdle(sessions, "/r", ""); p != "%3" || n != "free" {
		t.Errorf("pickIdle = %s %s, want the idle agent in the repo", p, n)
	}
	if p, _ := pickIdle(sessions, "/rr", ""); p != "" {
		t.Errorf("a sibling dir /rr is not inside /r, got %s", p)
	}
}
