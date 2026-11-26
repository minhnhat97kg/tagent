package tmux

import (
	"strings"
	"testing"
)

func TestNickname(t *testing.T) {
	n := len(nicknames)
	seen := map[string]bool{}
	for _, w := range nicknames {
		if seen[w] {
			t.Fatalf("duplicate nickname %q", w)
		}
		seen[w] = true
	}
	if a, b := nickname(7, nil), nickname(7+n, nil); a != b || a != nicknames[7] {
		t.Fatalf("not deterministic: %q %q", a, b)
	}
	used := map[string]bool{nicknames[n-1]: true}
	if got := nickname(n-1, used); got != nicknames[0] {
		t.Fatalf("taken name: got %q", got)
	}
	if got := nickname(0, seen); got != nicknames[0]+"-2" {
		t.Fatalf("pool used up: got %q", got)
	}
}

func TestResolvePaneChecked(t *testing.T) {
	if _, ok := RunTmux("new-session", "-d", "-s", "nick", "sleep 60"); !ok {
		t.Skip("no tmux")
	}
	defer RunTmux("kill-session", "-t", "nick")
	ids, _ := RunTmux("list-panes", "-t", "nick", "-F", "#{pane_id}")
	RunTmux("split-window", "-t", "nick", "sleep 60")
	first := strings.Fields(ids)[0]
	SetPaneOption(first, PaneAgent, "claude")
	sessions := QuerySessions()
	if len(sessions) != 1 {
		t.Fatalf("sessions: %v", sessions)
	}
	name := sessions[0].Windows[0].Panes[0].Name
	if name == "" || PaneLabel(first) != name || QuerySessions()[0].Windows[0].Panes[0].Name != name {
		t.Fatalf("nickname not stored: %q", name)
	}
	if id, err := ResolvePaneChecked(name); err != nil || id != first {
		t.Fatalf("by name: %q %v", id, err)
	}
	if _, err := ResolvePaneChecked("nobody"); err == nil {
		t.Fatal("unknown name must fail")
	}
	for _, target := range []string{first, "@0", "nick:0"} {
		if id, err := ResolvePaneChecked(target); err != nil || id != target {
			t.Fatalf("pass through %q: %q %v", target, id, err)
		}
	}
	all, _ := RunTmux("list-panes", "-t", "nick", "-F", "#{pane_id}")
	for _, id := range strings.Fields(all) {
		SetPaneOption(id, PaneName, "twin")
	}
	if _, err := ResolvePaneChecked("twin"); err == nil {
		t.Fatal("ambiguous name must fail")
	}
}
