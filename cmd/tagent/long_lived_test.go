package main

import (
	"strings"
	"tagent/internal/board"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"testing"
)

// A long_lived role keys one pane per role, so the next ticket reuses it.
func TestLongLivedPaneReusedAcrossTickets(t *testing.T) {
	defer tmux.SetRunner(tmux.NewFake())()
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	standing := board.Action{Dir: dir, Agent: "dev", LongLived: true, Event: tasks.Event{Task: "T2"}}
	if key := boardPaneKey(standing); key != "dev#" {
		t.Fatal("standing key carries a ticket:", key)
	}
	sessions := []tmux.SessionInfo{{Windows: []tmux.WindowInfo{{Panes: []tmux.PaneInfo{
		{PaneID: "%5", BoardTarget: "dev#", Agent: "pi", Path: dir},
	}}}}}
	if pane := findBoardPane(sessions, scope, standing, boardPaneKey(standing)); pane != "%5" {
		t.Fatal("standing pane not reused:", pane)
	}

	// A ticketed (non-long_lived) role still keeps one pane per ticket.
	perTicket := board.Action{Dir: dir, Agent: "dev", Event: tasks.Event{Task: "T2"}}
	if key := boardPaneKey(perTicket); key != "dev#T2" {
		t.Fatal("per-ticket key:", key)
	}
	if pane := findBoardPane(sessions, scope, perTicket, boardPaneKey(perTicket)); pane != "" {
		t.Fatal("per-ticket action reused the standing pane:", pane)
	}
}

// Closing a done ticket kills its own role panes and leaves the standing pane.
func TestLongLivedPaneSurvivesTicketClose(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	fake.Set("%30|dev#||"+dir+"\n%31|dev#T1||"+dir,
		"list-panes", "-a", "-F", "#{pane_id}|#{q:@board_target}|#{q:@pane_cwd}|#{q:pane_current_path}")
	fake.Set("", "kill-pane", "-t", "%31")
	if err := dispatchBoardAction(board.Action{Dir: dir, Close: true, Event: tasks.Event{Task: "T1"}}); err != nil {
		t.Fatal(err)
	}
	var killed []string
	for _, call := range fake.Calls() {
		if call[0] == "kill-pane" {
			killed = append(killed, call[len(call)-1])
		}
	}
	if strings.Join(killed, ",") != "%31" {
		t.Fatal("wrong panes closed", killed)
	}
}
