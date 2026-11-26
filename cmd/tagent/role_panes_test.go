package main

import "testing"

// Test binaries point TMUX_TMPDIR at an empty directory, so no server is up.
func TestRolePanesWithoutTmuxServer(t *testing.T) {
	panes, err := boardRolePanes(t.TempDir(), "dev")
	if err != nil || len(panes) != 0 {
		t.Fatalf("no tmux server must count as zero panes: %v %v", panes, err)
	}
}
