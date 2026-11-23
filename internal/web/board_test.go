package web

import (
	"context"
	"testing"
	"time"

	"tagent/internal/board"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func TestWebDiscoversBoardWithoutBrowser(t *testing.T) {
	defer tmux.SetRunner(tmux.NewFake())()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	if err := tasks.SaveBoardConfig(dir, []byte("triggers: [{on: create, agent: planner}]")); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Add(tasks.Scope(dir), "request", nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	fired := make(chan board.Action, 2)
	s := server{dir: dir}
	go func() {
		defer close(done)
		s.discoverBoards(ctx, func(a board.Action) error { fired <- a; return nil })
	}()
	select {
	case a := <-fired:
		if a.Agent != "planner" {
			t.Fatal(a)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no board dispatcher")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("discovery did not stop")
	}
}
