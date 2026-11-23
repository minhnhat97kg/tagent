package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"tmux-agent/internal/board"
	"tmux-agent/internal/tasks"
	"tmux-agent/internal/tmux"
)

func TestRunClosesWorkflowPanesWithoutAgentMetadata(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	foreign := t.TempDir()
	scope := tasks.Scope(dir)
	path := filepath.Join(dir, ".tasks", "workflows")
	if e := os.MkdirAll(path, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(path, "research.yml"), []byte("name: research\njobs:\n  research: {status: researching, agent: researcher}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	ticket, e := tasks.Add(scope, "research", nil, tasks.MutationOptions{Workflow: "research", Actor: "lead"})
	if e != nil {
		t.Fatal(e)
	}
	if e = tasks.Move(scope, ticket.ID, "researching", "", tasks.MutationOptions{Actor: "lead"}); e != nil {
		t.Fatal(e)
	}
	if e = tasks.Move(scope, ticket.ID, "done", "finished", tasks.MutationOptions{Actor: "researcher"}); e != nil {
		t.Fatal(e)
	}
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	// No @pane_agent: QuerySessions would omit the owned pane entirely.
	fake.Set("%26|researcher#"+ticket.ID+"||"+dir+"\n%27|reviewer#"+ticket.ID+"|"+dir+"|/elsewhere\n%28|researcher#"+ticket.ID+"||"+foreign+"\n%29|researcher#other||"+dir,
		"list-panes", "-a", "-F", "#{pane_id}|#{q:@board_target}|#{q:@pane_cwd}|#{q:pane_current_path}")
	fake.Set("", "kill-pane", "-t", "%26")
	fake.Set("", "kill-pane", "-t", "%27")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	closes := 0
	e = board.Run(ctx, dir, func(a board.Action) error {
		if !a.Close {
			return nil
		}
		closes++
		defer cancel()
		if a.Event.Actor != "researcher" {
			t.Fatal(a.Event)
		}
		return dispatchBoardAction(a)
	})
	if e != nil || closes != 1 {
		t.Fatal(e, closes)
	}
	var killed []string
	for _, call := range fake.Calls() {
		if call[0] == "kill-pane" {
			killed = append(killed, call[len(call)-1])
		}
	}
	if strings.Join(killed, ",") != "%26,%27" {
		t.Fatal("wrong panes closed", killed)
	}
	// Consumption survives restart: no replay of the close event.
	ctx2, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if e = board.Run(ctx2, dir, func(a board.Action) error { t.Fatal("replayed event", a); return nil }); e != nil {
		t.Fatal(e)
	}
}
