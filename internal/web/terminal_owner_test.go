package web

import (
	"slices"
	"strings"
	"testing"
)

type ownerFake struct {
	options map[string]string
	calls   []string
	policy  string
}

func (f *ownerFake) run(a ...string) (string, bool) {
	f.calls = append(f.calls, strings.Join(a, " "))
	pane := ""
	for i := 0; i+1 < len(a); i++ {
		if a[i] == "-t" {
			pane = a[i+1]
		}
	}
	if a[0] == "display-message" {
		return f.options[pane+strings.TrimSuffix(strings.TrimPrefix(a[len(a)-1], "#{"), "}")], true
	}
	if a[0] == "show-options" {
		return f.policy, true
	}
	if a[0] == "set-option" {
		for i, s := range a {
			if strings.HasPrefix(s, "@") {
				value := ""
				if i+1 < len(a) {
					value = a[i+1]
				}
				f.options[pane+s] = value
			}
		}
	}
	return "", true
}
func TestTerminalOwnerTakeoverRelease(t *testing.T) {
	f := ownerFake{options: map[string]string{}, policy: "latest"}
	first, second := terminalOwners{run: f.run}, terminalOwners{run: f.run}
	size := terminalSize{Cols: 80, Rows: 24}
	_ = first.resize("%1", "one", size, false, false)
	if f.options["%1@agent_size_owner"] != "" {
		t.Fatal("attached viewer claimed without handoff")
	}
	if err := first.resize("%1", "one", size, true, false); err != nil {
		t.Fatal(err)
	}
	if f.options["%1@agent_size_owner"] != "web:one" || !first.owns("%1", "one") {
		t.Fatal("owner option not set")
	}
	if err := second.resize("%1", "two", size, true, false); err != nil {
		t.Fatal(err)
	}
	if first.owns("%1", "one") || !second.owns("%1", "two") {
		t.Fatal("cross-server takeover failed")
	}
	n := len(f.calls)
	_ = first.resize("%1", "one", size, false, true)
	_ = first.release("%1", "one")
	for _, call := range f.calls[n:] {
		if strings.HasPrefix(call, "resize-window") || strings.HasPrefix(call, "set-option") {
			t.Fatalf("stale owner mutated tmux: %s", call)
		}
	}
	if err := second.release("%1", "two"); err != nil {
		t.Fatal(err)
	}
	if f.options["%1@agent_size_owner"] != "" {
		t.Fatal("owner option not cleared")
	}
	if !strings.Contains(strings.Join(f.calls, "\n"), "resize-window -A -t %1") {
		t.Fatal("release did not recompute window")
	}
	if !strings.Contains(strings.Join(f.calls, "\n"), "window-size latest") {
		t.Fatal("policy not restored")
	}
}
func TestTerminalOwnerExternalClear(t *testing.T) {
	f := ownerFake{options: map[string]string{}}
	o := terminalOwners{run: f.run}
	size := terminalSize{Cols: 80, Rows: 24}
	_ = o.resize("%1", "viewer", size, true, false)
	f.options["%1@agent_size_owner"] = "" // CLI/TUI returns sizing to the terminal
	if o.owns("%1", "viewer") {
		t.Fatal("external clear ignored")
	}
	n := len(f.calls)
	_ = o.release("%1", "viewer")
	if len(f.calls) != n+1 {
		t.Fatal("stale disconnect restored sizing")
	}
}
func TestTerminalOwnerRestoresInheritedPolicy(t *testing.T) {
	f := ownerFake{options: map[string]string{}}
	o := terminalOwners{run: f.run}
	_ = o.resize("%1", "viewer", terminalSize{Cols: 80, Rows: 24}, true, false)
	_ = o.release("%1", "viewer")
	if !strings.Contains(strings.Join(f.calls, "\n"), "set-option -w -u -t %1 window-size") {
		t.Fatal("inherited policy not restored")
	}
}
func TestTerminalAutoOwnerAndFailedClaim(t *testing.T) {
	f := ownerFake{options: map[string]string{}}
	o := terminalOwners{run: f.run}
	size := terminalSize{Cols: 80, Rows: 24}
	_ = o.resize("%1", "first", size, false, true)
	_ = o.resize("%1", "second", size, false, true)
	if !o.owns("%1", "first") {
		t.Fatal("passive auto-resize stole ownership")
	}
	bad := terminalOwners{run: func(...string) (string, bool) { return "", false }}
	if err := bad.resize("%1", "v", size, true, false); err == nil || bad.owns("%1", "v") {
		t.Fatal("failed claim accepted")
	}
}

func TestKeepZoomedRezoomsOwnerPane(t *testing.T) {
	var calls []string
	zoom := "0"
	o := &terminalOwners{run: func(args ...string) (string, bool) {
		calls = append(calls, strings.Join(args, " "))
		switch {
		case args[0] == "display-message" && strings.HasSuffix(args[len(args)-1], "@agent_size_owner}"):
			return "web:v1\n", true
		case args[0] == "display-message":
			return "3 " + zoom + "\n", true
		}
		return "", true
	}}
	o.keepZoomed("%1", "other") // not the owner: nothing happens
	if len(calls) != 1 {
		t.Fatalf("a viewer that is not the owner must not touch zoom: %v", calls)
	}
	calls = nil
	zoom = "1"
	o.keepZoomed("%1", "v1") // already zoomed
	for _, c := range calls {
		if strings.HasPrefix(c, "resize-pane") {
			t.Fatalf("already zoomed pane toggled off: %v", calls)
		}
	}
	calls = nil
	zoom = "0"
	o.keepZoomed("%1", "v1")
	if !slices.Contains(calls, "resize-pane -Z -t %1") || !slices.Contains(calls, "set-option -p -t %1 @agent_size_zoomed 1") {
		t.Fatalf("lost zoom was not restored: %v", calls)
	}
}
