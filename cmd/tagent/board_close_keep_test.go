package main

import (
	"os"
	"path/filepath"
	"tagent/internal/board"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"testing"
)

// Done closes a ticket's panes, except when its worktree is kept for uncommitted
// changes: the agents may still be working in it.
func TestDoneCloseKeepsPanesOfKeptWorktree(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@test")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@test")
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"commit", "--allow-empty", "-qm", "baseline"}} {
		if _, err := git(repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	wt, err := workflowWorktree(repo, "7")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "wip"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	fake.Set("%41|dev#7||"+repo, "list-panes", "-a", "-F", "#{pane_id}|#{q:@board_target}|#{q:@pane_cwd}|#{q:pane_current_path}")
	if err := dispatchBoardAction(board.Action{Dir: repo, Close: true, Event: tasks.Event{Task: "7"}}); err != nil {
		t.Fatal(err)
	}
	for _, call := range fake.Calls() {
		if call[0] == "kill-pane" {
			t.Fatalf("kept worktree's pane closed: %v", call)
		}
	}
}
