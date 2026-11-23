package tmux

import (
	"errors"
	"testing"
)

func TestBoardPanesReadsOwnershipWithoutDetection(t *testing.T) {
	f := NewFake()
	defer SetRunner(f)()
	args := []string{"list-panes", "-a", "-F", "#{pane_id}|#{q:@board_target}|#{q:@pane_cwd}|#{q:pane_current_path}"}
	f.Set("%26|researcher#ticket||/repo\n%27|reviewer#ticket|/repo\\|name|/shell\n%28|||/repo\n", args...)
	panes, e := BoardPanes()
	if e != nil || len(panes) != 2 {
		t.Fatal(panes, e)
	}
	if panes[0].Path != "/repo" || panes[1].Path != "/repo|name" || panes[1].BoardTarget != "reviewer#ticket" {
		t.Fatal(panes)
	}
	// No process snapshot or detector query, and tmux failures are not hidden.
	if len(f.Calls()) != 1 {
		t.Fatal(f.Calls())
	}
	f.SetErr(errors.New("server unavailable"), args...)
	if _, e = BoardPanes(); e == nil {
		t.Fatal("query failure hidden")
	}
}
