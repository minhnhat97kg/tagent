package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/board"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workplace"
)

// A `workplace: browser` ticket runs in the main root: no worktree, no branch. Its
// dispatch prompt names the captures and profile dirs. Done attaches the captures
// and removes the ticket's browser dir.
func TestWorkplaceBrowserRunsInMainRoot(t *testing.T) {
	dir, scope, ticket := startWorkplaceTicket(t, "workplace: browser\n")
	wp, err := tasks.TicketWorkplace(scope, ticket)
	b, ok := wp.(workplace.Browser)
	if !ok || err != nil || b.Keep {
		t.Fatalf("browser workflow: %#v %v", wp, err)
	}
	captures, profile := b.Output(dir, ticket.ID), b.Profile(ticket.ID)

	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0700)
	a := board.Action{Dir: sub, Agent: "planner", Prompt: "first prompt"}
	a.Event.Task = ticket.ID
	if err := dispatchBoardAction(a); err != nil {
		t.Fatal(err)
	}
	name := "planner-" + ticket.ID[max(0, len(ticket.ID)-4):]
	pane := tmux.ResolvePane(name)
	if pane == name {
		t.Fatal("pane not started")
	}
	found := false
	for _, s := range tmux.QuerySessions() {
		for _, w := range s.Windows {
			for _, p := range w.Panes {
				if p.PaneID == pane {
					found = true
					if p.Path != dir {
						t.Fatalf("pane cwd %s, want main root %s", p.Path, dir)
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("pane not listed")
	}
	for _, d := range []string{captures, profile} {
		if _, err := os.Stat(d); err != nil {
			t.Fatalf("dispatch did not create %s: %v", d, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, workplace.Dir)); !os.IsNotExist(err) {
		t.Fatalf(".worktrees: %v", err)
	}
	if out, err := git(dir, "branch", "--list", "agent/workflow-*"); err != nil || out != "" {
		t.Fatalf("workflow branches: %q %v", out, err)
	}
	events, _ := tasks.ReadEvents(scope, 0)
	prompted := false
	for _, e := range events {
		if e.Kind == "dispatch" && strings.Contains(e.Prompt, captures) && strings.Contains(e.Prompt, profile) {
			prompted = true
		}
	}
	if !prompted {
		t.Fatalf("dispatch prompt does not name %s and %s", captures, profile)
	}

	os.WriteFile(filepath.Join(captures, "shot.png"), []byte("s"), 0600)
	os.WriteFile(filepath.Join(profile, "Cookies"), []byte("c"), 0600)
	if err := tasks.Move(scope, ticket.ID, "done", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(captures)); !os.IsNotExist(err) {
		t.Fatalf("browser dir kept after done: %v", err)
	}
	list, err := tasks.AttachmentList(scope, dir, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	gotCapture, gotProfile := false, false
	for _, a := range list {
		if a.Source == "comment" && a.Error == "" {
			gotCapture = gotCapture || a.Name == "shot.png"
			gotProfile = gotProfile || a.Name == "Cookies"
		}
	}
	if !gotCapture || gotProfile {
		t.Fatalf("want shot.png attached and Cookies not: %+v", list)
	}
}
