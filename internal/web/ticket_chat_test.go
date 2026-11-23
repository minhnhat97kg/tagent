package web

import (
	"encoding/json"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func TestTicketChatAPIFindsBoundPane(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	ticket, err := tasks.Add(tasks.Scope(dir), "chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	key := tasks.TicketScope(dir, ticket.ID)
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	fake.Set("%4|"+key+"|pi|idle|"+dir+"\n", "list-panes", "-a", "-F",
		"#{pane_id}|#{q:@pane_ticket}|#{q:@pane_agent}|#{q:@pane_status}|#{q:pane_current_path}")
	s := &server{token: "secret", dir: dir}
	w := request(s, "GET", "/api/tickets/"+ticket.ID+"/chat", "", true)
	var got ticketChatJSON
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err, w.Body.String())
	}
	if w.Code != 200 || !got.Available || got.Pane != "%4" || got.Harness != "pi" || got.Key != key {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestTicketChatAPIReportsUnsupportedHarness(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	ticket, err := tasks.Add(tasks.Scope(dir), "chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	key := tasks.TicketScope(dir, ticket.ID)
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	fake.Set("%5|"+key+"|codex|idle|"+dir+"\n", "list-panes", "-a", "-F",
		"#{pane_id}|#{q:@pane_ticket}|#{q:@pane_agent}|#{q:@pane_status}|#{q:pane_current_path}")
	w := request(&server{token: "secret", dir: dir}, "GET", "/api/tickets/"+ticket.ID+"/chat", "", true)
	var got ticketChatJSON
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err, w.Body.String())
	}
	if got.Available || got.Reason == "" || got.Pane != "%5" {
		t.Fatalf("%s", w.Body.String())
	}
}

func TestTicketChatAPIMissingTicket(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	w := request(&server{token: "secret", dir: dir}, "GET", "/api/tickets/404/chat", "", true)
	if w.Code != 404 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
