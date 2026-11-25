package main

import (
	"bytes"
	"strings"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

// Purge --yes deletes work, so an agent pane may not run it; a dry run is fine.
func TestPurgeYesRefusedFromAgentPane(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%9")
	dir := t.TempDir()
	t.Chdir(dir)
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	fake.Set("pi", "show", "-t", "%9", "-pv", tmux.PaneAgent)
	var out, stderr bytes.Buffer
	if code := cmdTask([]string{"purge", "--closed", "--yes"}, &out, &stderr); code != exitFailure || !strings.Contains(stderr.String(), "human pane") {
		t.Fatalf("code %d: %s", code, stderr.String())
	}
	if code := cmdTask([]string{"purge", "--closed"}, &out, &stderr); code != exitOK {
		t.Fatalf("dry run refused: %s", stderr.String())
	}
}

func TestCloseCommandClosesTicket(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	t.Chdir(dir)
	ticket, err := tasks.Add(tasks.Scope(dir), "drop me", nil)
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := cmdTask([]string{"close", ticket.ID, "dropped"}, &out, &stderr); code != exitOK {
		t.Fatalf("close: %s", stderr.String())
	}
	if got, _ := tasks.Get(tasks.Scope(dir), ticket.ID); got.Status != "closed" {
		t.Fatalf("status %q", got.Status)
	}
}

// Purge prints one line per closed ticket; the dry run says nothing is removed, --yes removes.
func TestPurgeCLIOutput(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	t.Chdir(dir)
	ticket, err := tasks.Add(tasks.Scope(dir), "Drop me", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Move(tasks.Scope(dir), ticket.ID, "closed", ""); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := cmdTask([]string{"purge", "--closed"}, &out, &stderr); code != exitOK {
		t.Fatal(stderr.String())
	}
	if got := out.String(); !strings.Contains(got, "remove #"+ticket.ID+" Drop me\n") || !strings.Contains(got, "nothing removed") {
		t.Fatalf("dry run output: %q", got)
	}
	out.Reset()
	if code := cmdTask([]string{"purge", "--closed", "--yes"}, &out, &stderr); code != exitOK {
		t.Fatal(stderr.String())
	}
	if got := out.String(); !strings.Contains(got, "removed #"+ticket.ID+" Drop me\n") {
		t.Fatalf("yes output: %q", got)
	}
	if _, err := tasks.Get(tasks.Scope(dir), ticket.ID); err == nil {
		t.Fatal("ticket kept")
	}
}

// task start on a closed ticket is refused before any pane is linked or moved.
func TestStartRefusesClosedTicket(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	t.Chdir(dir)
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "dropped", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Move(scope, ticket.ID, "closed", ""); err != nil {
		t.Fatal(err)
	}
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	var out, stderr bytes.Buffer
	if code := cmdTask([]string{"start", ticket.ID, "%7"}, &out, &stderr); code != exitFailure || !strings.Contains(stderr.String(), "is closed") {
		t.Fatalf("code %d: %s", code, stderr.String())
	}
	if got, _ := tasks.Get(scope, ticket.ID); got.Status != "closed" {
		t.Fatalf("status %q", got.Status)
	}
}
