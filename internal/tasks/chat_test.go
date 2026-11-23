package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/tmux"
)

func TestTicketScopeIsProjectScopePlusTicket(t *testing.T) {
	dir := t.TempDir()
	if got, want := TicketScope(dir, "12"), Scope(dir)+"#12"; got != want {
		t.Fatal(got, want)
	}
}

func TestContextForRendersTicketState(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	if err := SaveBoardConfig(dir, []byte(`statuses:
  - {id: todo, next: [doing]}
  - {id: doing, role: builder, next: [done, blocked]}
  - {id: blocked}
  - {id: done}
`)); err != nil {
		t.Fatal(err)
	}
	scope := Scope(dir)
	ticket, err := Add(scope, "Fix the export\nLonger description of the bug", []string{"alice"})
	if err != nil {
		t.Fatal(err)
	}
	if err = SetNote(scope, ticket.ID, "started on the parser"); err != nil {
		t.Fatal(err)
	}
	if _, err = Update(scope, ticket.ID, "doing", []string{"alice", "bob"}); err != nil {
		t.Fatal(err)
	}
	patch := filepath.Join(t.TempDir(), "fix.patch")
	if err = os.WriteFile(patch, []byte("diff"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = Comment(scope, ticket.ID, "planner", "handing off to @implementer", []string{patch}); err != nil {
		t.Fatal(err)
	}

	got, err := ContextFor(scope, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		ticket.ID + ": Fix the export",
		"Status: doing",
		"Step: doing (role builder); next: done, blocked",
		"task show " + ticket.ID,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("context missing %q:\n%s", want, got)
		}
	}
	// The brief is a summary: no per-field dump beyond Brief.
	for _, bad := range []string{"Assignees:", "Note:", "Due:", "Input "} {
		if strings.Contains(got, bad) {
			t.Fatalf("context should not contain %q:\n%s", bad, got)
		}
	}
	if _, err = ContextFor(scope, "does-not-exist"); err == nil {
		t.Fatal("unknown ticket accepted")
	}
}

func TestContextForWorkflowStep(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".tasks", "workflows"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tasks", "workflows", "ship.yaml"), []byte(`name: ship
inputs:
  env: {default: prod}
jobs:
  build:
    status: build
    agent: builder
    prompt: build it
`), 0600); err != nil {
		t.Fatal(err)
	}
	scope := Scope(dir)
	ticket, err := Add(scope, "ship it", nil, MutationOptions{Workflow: "ship"})
	if err != nil {
		t.Fatal(err)
	}
	if err = Move(scope, ticket.ID, "build", ""); err != nil {
		t.Fatal(err)
	}
	got, err := ContextFor(scope, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Status: build", "Step: build (role builder) (job build); next: done, blocked"} {
		if !strings.Contains(got, want) {
			t.Fatalf("context missing %q:\n%s", want, got)
		}
	}
}

func TestCallerTicketReadsPaneBinding(t *testing.T) {
	t.Setenv("TMUX_PANE", "%26")
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	fake.Set("repo-1234#12", "show", "-t", "%26", "-pv", tmux.PaneTicket)
	scope, id, ok := CallerTicket()
	if !ok || scope != "repo-1234" || id != "12" {
		t.Fatal(scope, id, ok)
	}
	t.Setenv("TMUX_PANE", "")
	if _, _, ok = CallerTicket(); ok {
		t.Fatal("unbound pane reported a ticket")
	}
}
