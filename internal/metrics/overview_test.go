package metrics

import (
	"testing"
	"time"
	"tmux-agent/internal/tasks"
	"tmux-agent/internal/tmux"
)

func TestTokenWindow(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local)
	s := State{Sessions: map[string]Session{"a": {Daily: map[string]int64{"2026-01-01": 7, "2026-01-02": 14}}, "b": {Daily: map[string]int64{"2026-01-02": 7}}}}
	w := TokenWindow(s, now, 7)
	if len(w.Days) != 7 || w.Days[0].Date != "2025-12-27" || w.Today != 21 || w.Total != 28 || len([]rune(w.Sparkline)) != 7 {
		t.Fatalf("%+v", w)
	}
	for _, days := range []int{1, 7, 30} {
		w = TokenWindow(State{}, now, days)
		if len(w.Days) != days || w.Total != 0 {
			t.Fatal(w)
		}
	}
}
func TestOverviewFacts(t *testing.T) {
	a, n := OverviewFacts([]tmux.PaneInfo{{Name: "waiting", Status: tmux.StatusWaiting}, {Name: "quiet", Status: tmux.StatusRunning, Stuck: "5"}, {Name: "idle", Status: tmux.StatusIdle, Stuck: "5"}}, []tasks.Task{{Status: "blocked"}, {Status: "doing", Text: "one\ntwo"}}, State{})
	if !a.Needed || len(a.Items) != 3 || len(n) != 1 || n[0] != "one two" {
		t.Fatalf("%+v %v", a, n)
	}
	a, n = OverviewFacts(nil, nil, State{})
	if a.Needed || a.Text != "nothing needs you" || a.Items == nil || n == nil {
		t.Fatal(a, n)
	}
}
