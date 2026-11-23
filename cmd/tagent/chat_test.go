package main

import (
	"bytes"
	"strings"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func TestChatReusesBoundPane(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "chat me", nil)
	if err != nil {
		t.Fatal(err)
	}
	key := tasks.TicketScope(dir, ticket.ID)
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	fake.Set("%3|"+key+"|claude|idle|"+dir+"\n", "list-panes", "-a", "-F",
		"#{pane_id}|#{q:@pane_ticket}|#{q:@pane_agent}|#{q:@pane_status}|#{q:pane_current_path}")
	var out, stderr bytes.Buffer
	if code := cmdChat([]string{"--dir", dir, "--agent", "claude", ticket.ID}, &out, &stderr); code != exitOK {
		t.Fatal(code, stderr.String())
	}
	if strings.TrimSpace(out.String()) != "%3" {
		t.Fatal(out.String())
	}
}

func TestTaskFromChatPaneIsBoundToItsTicket(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "%9")
	dir := t.TempDir()
	t.Chdir(dir)
	scope := tasks.Scope(dir)
	mine, err := tasks.Add(scope, "mine", nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := tasks.Add(scope, "other", nil)
	if err != nil {
		t.Fatal(err)
	}
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	fake.Set(tasks.TicketScope(dir, mine.ID), "show", "-t", "%9", "-pv", tmux.PaneTicket)

	var out, stderr bytes.Buffer
	if code := cmdTask([]string{"comment", other.ID, "should not land"}, &out, &stderr); code != exitFailure {
		t.Fatalf("other ticket accepted: %d %s", code, out.String())
	}
	if got, _ := tasks.Get(scope, other.ID); len(got.Comments) != 0 {
		t.Fatalf("comment landed on another ticket: %+v", got.Comments)
	}
	if !strings.Contains(stderr.String(), mine.ID) {
		t.Fatalf("refusal does not name the bound ticket: %s", stderr.String())
	}
	out.Reset()
	stderr.Reset()
	if code := cmdTask([]string{"comment", mine.ID, "mine"}, &out, &stderr); code != exitOK {
		t.Fatalf("own ticket refused: %d %s", code, stderr.String())
	}
	if got, _ := tasks.Get(scope, mine.ID); len(got.Comments) != 1 || got.Comments[0].Body != "mine" {
		t.Fatalf("comment missing: %+v", got)
	}
	if code := cmdTask([]string{"add", "new ticket"}, &out, &stderr); code != exitFailure {
		t.Fatal("a chat pane could create a ticket")
	}
	if code := cmdTask([]string{"ls"}, &out, &stderr); code != exitOK {
		t.Fatal("read-only listing refused")
	}
}
