package metrics

import (
	"testing"
	"time"

	"tagent/internal/tasks"
	"tagent/internal/usage"
)

func TestTicketUsageOf(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	disp := func(role, harness, model, name, pane string, at time.Duration) tasks.Event {
		return tasks.Event{Task: "7", Kind: "dispatch", Time: t0.Add(at), Role: role, Harness: harness, Model: model, PaneName: name, Pane: pane}
	}
	sess := func(key, harness, name, pane, status string, total int64, has bool) Session {
		return Session{Key: key, Harness: harness, Agent: name, Pane: pane, Status: status, FirstSeen: t0, HasUsage: has, Usage: usage.Count{Total: total, Input: total}}
	}
	state := State{Sessions: map[string]Session{
		"pi:a":   sess("pi:a", "pi", "dev-7", "%1", "closed", 100, true),
		"pi:b":   sess("pi:b", "pi", "qa-7", "%2", "closed", 40, true),
		"ag:c":   sess("ag:c", "antigravity", "doc-7", "%3", "closed", 0, false),
		"pi:old": {Key: "pi:old", Harness: "pi", Agent: "dev-7", Pane: "%9", Status: "closed", FirstSeen: t0.Add(-48 * time.Hour), HasUsage: true, Usage: usage.Count{Total: 5000}},
	}}
	events := []tasks.Event{
		disp("dev", "pi", "gpt", "dev-7", "%1", 0),
		disp("qa", "pi", "gpt", "qa-7", "%2", time.Minute),
		disp("dev", "pi", "gpt", "dev-7", "%1", 2*time.Minute), // re-entered job, same session
		disp("doc", "antigravity", "g", "doc-7", "%3", 3*time.Minute),
		{Task: "other", Kind: "dispatch", Role: "dev", PaneName: "dev-7"},
	}
	u := TicketUsageOf(state, tasks.Task{ID: "7"}, tasks.BoardConfig{}, events, t0.Add(time.Hour))
	if len(u.Jobs) != 3 || u.Loops != 4 {
		t.Fatalf("jobs=%d loops=%d", len(u.Jobs), u.Loops)
	}
	if j := u.Jobs[0]; j.Role != "dev" || j.Loops != 2 || j.Count.Total != 100 || j.Live {
		t.Fatalf("re-entered job counted wrong: %+v", j)
	}
	if u.Total.Total != 140 || !u.Partial {
		t.Fatalf("total=%d partial=%v, want 140 partial (n/a job excluded)", u.Total.Total, u.Partial)
	}
	if j := u.Jobs[2]; j.Has {
		t.Fatalf("harness without usage must be n/a: %+v", j)
	}
	if len(u.Models) != 2 || u.Models[0].Model != "gpt" || u.Models[0].Count.Total != 140 {
		t.Fatalf("models=%+v", u.Models)
	}
	// pane id fallback when the name is unknown, and legacy runs.
	state.Runs = []Run{{TaskID: "7", SessionKey: "legacy", Agent: "x", Started: t0, Ended: t0, HasStart: true, HasEnd: true, EndTotal: 7}}
	u = TicketUsageOf(state, tasks.Task{ID: "7"}, tasks.BoardConfig{}, []tasks.Event{disp("dev", "pi", "gpt", "", "%2", 0)}, t0)
	if u.Jobs[0].Count.Total != 40 || len(u.Jobs) != 2 || u.Total.Total != 47 {
		t.Fatalf("fallbacks: %+v", u)
	}
	// a named dispatch never falls back to a reused pane id of another ticket.
	u = TicketUsageOf(state, tasks.Task{ID: "7"}, tasks.BoardConfig{}, []tasks.Event{disp("dev", "pi", "gpt", "renamed", "%2", 0)}, t0)
	if u.Jobs[0].Has || u.Total.Total != 7 {
		t.Fatalf("pane id reuse attributed: %+v", u)
	}
	u = TicketUsageOf(state, tasks.Task{ID: "7"}, tasks.BoardConfig{}, []tasks.Event{disp("dev", "pi", "gpt", "", "%2", 0)}, t0)
	if got := u.Lines(func(n int64) string { return "N" }); len(got) != 5 {
		t.Fatalf("lines=%v", got)
	}
}
