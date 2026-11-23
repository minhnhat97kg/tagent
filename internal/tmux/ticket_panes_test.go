package tmux

import (
	"errors"
	"testing"
)

func TestTicketPanesReadsBindingWithoutDetection(t *testing.T) {
	f := NewFake()
	defer SetRunner(f)()
	args := []string{"list-panes", "-a", "-F", "#{pane_id}|#{q:@pane_ticket}|#{q:@pane_agent}|#{q:@pane_status}|#{q:pane_current_path}"}
	f.Set("%26|repo-1234#12|claude|idle|/repo\n%27|repo-1234#12|pi|running|/repo\\|name\n%28||claude|idle|/repo\n", args...)
	panes, err := TicketPanes()
	if err != nil || len(panes) != 2 {
		t.Fatal(panes, err)
	}
	first, second := panes[0], panes[1]
	if first.PaneID != "%26" || first.Ticket != "repo-1234#12" || first.Agent != "claude" || first.Status != StatusIdle {
		t.Fatal(first)
	}
	if second.PaneID != "%27" || second.Agent != "pi" || second.Status != StatusRunning || second.Path != "/repo|name" {
		t.Fatal(second)
	}
	if len(f.Calls()) != 1 {
		t.Fatal(f.Calls())
	}
	f.SetErr(errors.New("server unavailable"), args...)
	if _, err = TicketPanes(); err == nil {
		t.Fatal("query failure hidden")
	}
}
