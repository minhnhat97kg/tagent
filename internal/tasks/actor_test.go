package tasks

import (
	"tagent/internal/tmux"
	"testing"
)

func TestCallerActorPaneMetadata(t *testing.T) {
	for _, tc := range []struct{ name, target, paneName, agent, want string }{
		{"role wins", "researcher#ticket", "researcher-1234", "pi", "researcher"},
		{"name without detector", "", "named-worker", "", "named-worker"},
		{"agent fallback", "", "", "pi", "pi"},
		{"malformed ownership", "researcher#", "named-worker", "pi", "named-worker"},
		{"no metadata", "", "", "", "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TMUX_PANE", "%26")
			f := tmux.NewFake()
			defer tmux.SetRunner(f)()
			for key, value := range map[string]string{tmux.BoardTarget: tc.target, tmux.PaneName: tc.paneName, tmux.PaneAgent: tc.agent} {
				f.Set(value, "show", "-t", "%26", "-pv", key)
			}
			if got := CallerActor(); got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
	t.Run("outside tmux", func(t *testing.T) {
		t.Setenv("TMUX_PANE", "")
		if CallerActor() != "user" {
			t.Fatal(CallerActor())
		}
	})
}
func TestDispatchedPaneAuthorsCommentsAndMoves(t *testing.T) {
	_, scope := testBoard(t)
	t.Setenv("TMUX_PANE", "%26")
	f := tmux.NewFake()
	defer tmux.SetRunner(f)()
	f.Set("researcher#ticket", "show", "-t", "%26", "-pv", tmux.BoardTarget)
	task, e := Add(scope, "work", nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = Comment(scope, task.ID, CallerActor(), "findings", nil); e != nil {
		t.Fatal(e)
	}
	if e = Move(scope, task.ID, "done", "finished"); e != nil {
		t.Fatal(e)
	}
	got, e := Get(scope, task.ID)
	if e != nil {
		t.Fatal(e)
	}
	for _, c := range got.Comments {
		if c.Author != "researcher" {
			t.Fatal(c)
		}
	}
	events, _ := ReadEvents(scope, 0)
	for _, ev := range events {
		if ev.Actor != "researcher" {
			t.Fatal(ev)
		}
	}
	if e = Comment(scope, task.ID, "system", "audit", nil, MutationOptions{Actor: "system"}); e != nil {
		t.Fatal(e)
	}
	events, _ = ReadEvents(scope, 0)
	if events[len(events)-1].Actor != "system" {
		t.Fatal("explicit actor lost")
	}
}
