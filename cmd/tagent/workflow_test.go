package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"tagent/internal/board"
	"tagent/internal/tasks"
	"testing"
)

func TestWorkflowWait(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	scope := tasks.Scope(t.TempDir())
	for _, status := range []string{"done", "blocked"} {
		ticket, e := tasks.Add(scope, "wait", nil)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = tasks.Update(scope, ticket.ID, status, nil); e != nil {
			t.Fatal(e)
		}
		want := exitFailure
		if status == "done" {
			want = exitOK
		}
		if got := waitWorkflow(context.Background(), scope, ticket.ID, io.Discard); got != want {
			t.Fatal(got, want)
		}
	}
	ticket, e := tasks.Add(scope, "cancel", nil)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitWorkflow(ctx, scope, ticket.ID, io.Discard) != exitFailure {
		t.Fatal("cancel accepted")
	}
}

func TestWorkflowCommandOutputs(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@test")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@test")
	for _, args := range [][]string{{"init", "-q"}, {"commit", "--allow-empty", "-qm", "baseline"}} {
		if _, err := git(dir, args...); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, ".tasks", "workflows")
	if e := os.MkdirAll(path, 0700); e != nil {
		t.Fatal(e)
	}
	recipe := `name: example
jobs:
  collect:
    run: |
      printf 'bad\n=value\nfile=first\nfile=last=value\n' >> "$TMUX_AGENT_OUTPUT"
  review:
    needs: collect
    agent: reviewer
    model: test-model
    prompt: 'Read ${{ jobs.collect.outputs.file }} for ${{ ticket.id }}'
    max-loops: 1
    on-failure: collect
`
	if e := os.WriteFile(filepath.Join(path, "example.yml"), []byte(recipe), 0600); e != nil {
		t.Fatal(e)
	}
	scope := tasks.Scope(dir)
	ticket, e := tasks.Add(scope, "title", nil, tasks.MutationOptions{Workflow: "example"})
	if e != nil {
		t.Fatal(e)
	}
	if e = tasks.Move(scope, ticket.ID, "collect", ""); e != nil {
		t.Fatal(e)
	}
	cfg, e := tasks.LoadBoardConfig(dir)
	if e != nil {
		t.Fatal(e)
	}
	actions := func() []board.Action {
		t.Helper()
		events, e := tasks.ReadEvents(scope, 0)
		if e != nil {
			t.Fatal(e)
		}
		task, e := tasks.Get(scope, ticket.ID)
		if e != nil {
			t.Fatal(e)
		}
		a, e := board.Actions(dir, cfg, events[len(events)-1], task)
		if e != nil {
			t.Fatal(e)
		}
		return a
	}
	a := actions()
	if len(a) != 1 {
		t.Fatal(a)
	}
	a[0].ExecDir, e = workflowWorktree(dir, ticket.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = runStatus(a[0]); e != nil {
		t.Fatal(e)
	}
	got, e := tasks.Get(scope, ticket.ID)
	if e != nil || got.Status != "review" || got.Outputs["collect"]["file"] != "last=value" {
		t.Fatal(got, e)
	}
	a = actions()
	if len(a) != 1 || !strings.Contains(a[0].Prompt, "Read last=value") || a[0].Model != "test-model" {
		t.Fatal(a)
	}
	if e = tasks.Move(scope, ticket.ID, "collect", "retry"); e != nil {
		t.Fatal(e)
	}
	if e = tasks.Move(scope, ticket.ID, "review", ""); e != nil {
		t.Fatal(e)
	}
	// max-loops 1 allows one hand-back: the review entry after it still runs.
	a = actions()
	if len(a) != 1 || a[0].BlockReason != "" {
		t.Fatal(a)
	}
	if e = tasks.Move(scope, ticket.ID, "collect", "retry again"); e != nil {
		t.Fatal(e)
	}
	a = actions()
	if len(a) != 1 || a[0].BlockReason == "" {
		t.Fatal(a)
	}
}
