package board

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"tagent/internal/tasks"
)

const slotConfig = `roles:
  planner: {agent: pi, max_instances: 1}
triggers:
  - on: create
    agent: planner
`

// fakePanes is the live pane table a test edits: ticket -> idle.
type fakePanes map[string]bool

func (f fakePanes) live(string, string) (map[string]bool, error) {
	out := map[string]bool{}
	for k, v := range f {
		out[k] = v
	}
	return out, nil
}

// runFor runs the dispatcher for a short time and returns the tickets it fired.
func runFor(t *testing.T, dir string, live fakePanes, fired *[]string) {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer stop()
	err := RunWith(ctx, dir, func(a Action) error {
		if a.Close { // closing a done ticket's panes is not a dispatch
			return nil
		}
		*fired = append(*fired, a.Event.Task)
		live[a.Event.Task] = false // the new pane is busy at once
		return nil
	}, live.live)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMaxInstancesWaitsAndReleases(t *testing.T) {
	dir, scope := setup(t, slotConfig)
	first, _ := tasks.Add(scope, "one", nil)
	second, _ := tasks.Add(scope, "two", nil)
	live := fakePanes{}
	var fired []string
	runFor(t, dir, live, &fired)
	if len(fired) != 1 || fired[0] != first.ID {
		t.Fatalf("fired %v, want only %s", fired, first.ID)
	}
	// The first pane is still busy: the second ticket waits and the reason is audited.
	runFor(t, dir, live, &fired)
	if len(fired) != 1 {
		t.Fatalf("second ticket fired while the slot was taken: %v", fired)
	}
	events, _ := tasks.ReadEvents(scope, 0)
	audited := 0
	for _, ev := range events {
		if ev.Kind == "dispatch" && strings.Contains(ev.Message, "waiting: role planner at max_instances 1 (running: #"+first.ID+")") {
			audited++
		}
	}
	if audited != 1 {
		t.Fatalf("waiting audited %d times, want once", audited)
	}
	// An idle pane holds no slot, so the waiting ticket starts.
	live[first.ID] = true
	runFor(t, dir, live, &fired)
	if len(fired) != 2 || fired[1] != second.ID {
		t.Fatalf("after idle: fired %v", fired)
	}
	// Nothing is dispatched twice.
	live[second.ID] = true
	runFor(t, dir, live, &fired)
	if len(fired) != 2 {
		t.Fatalf("replayed: %v", fired)
	}
}

func TestMaxInstancesRestartKeepsWaiting(t *testing.T) {
	dir, scope := setup(t, slotConfig)
	tasks.Add(scope, "one", nil)
	second, _ := tasks.Add(scope, "two", nil)
	live := fakePanes{}
	var fired []string
	runFor(t, dir, live, &fired)
	// A new dispatcher (restart) reads the persisted waiting entry.
	runFor(t, dir, live, &fired)
	if len(fired) != 1 {
		t.Fatalf("restart fired early: %v", fired)
	}
	delete(live, fired[0]) // the first pane was closed
	runFor(t, dir, live, &fired)
	runFor(t, dir, live, &fired)
	if len(fired) != 2 || fired[1] != second.ID {
		t.Fatalf("fired %v, want second once", fired)
	}
}

func TestMaxInstancesDropsStaleTicket(t *testing.T) {
	dir, scope := setup(t, slotConfig)
	tasks.Add(scope, "one", nil)
	gone, _ := tasks.Add(scope, "two", nil)
	live := fakePanes{}
	var fired []string
	runFor(t, dir, live, &fired)
	if err := tasks.Remove(scope, gone.ID); err != nil {
		t.Fatal(err)
	}
	delete(live, fired[0])
	runFor(t, dir, live, &fired)
	if len(fired) != 1 {
		t.Fatalf("removed ticket dispatched: %v", fired)
	}
	events, _ := tasks.ReadEvents(scope, 0)
	dropped := false
	for _, ev := range events {
		dropped = dropped || (ev.Kind == "dispatch" && strings.Contains(ev.Message, "dropped:"))
	}
	if !dropped {
		t.Fatal("drop not audited")
	}
}

func TestMaxInstancesOffFiresAsBefore(t *testing.T) {
	dir, scope := setup(t, strings.Replace(slotConfig, ", max_instances: 1", "", 1))
	tasks.Add(scope, "one", nil)
	tasks.Add(scope, "two", nil)
	live := fakePanes{}
	var fired []string
	runFor(t, dir, live, &fired)
	if len(fired) != 2 {
		t.Fatalf("no limit fired %v", fired)
	}
}

func TestSlotFullRules(t *testing.T) {
	cfg, err := tasks.ParseBoardConfig([]byte(slotConfig))
	if err != nil {
		t.Fatal(err)
	}
	a := Action{Agent: "planner", Trigger: 0}
	cases := []struct {
		name  string
		panes map[string]bool
		want  int
	}{
		{"free", map[string]bool{}, 0},
		{"own pane reused", map[string]bool{"me": false}, 0},
		{"idle other holds nothing", map[string]bool{"other": true}, 0},
		{"busy other holds the slot", map[string]bool{"other": false}, 1},
	}
	for _, tc := range cases {
		got, _, _ := slotFull("dir", cfg, a, "me", func(string, string) (map[string]bool, error) { return tc.panes, nil })
		if got != tc.want {
			t.Fatalf("%s: max %d, want %d", tc.name, got, tc.want)
		}
	}
	if got, _, _ := slotFull("dir", cfg, Action{Agent: "planner", Run: "go test"}, "me", func(string, string) (map[string]bool, error) { return map[string]bool{"x": false}, nil }); got != 0 {
		t.Fatal("run job waited")
	}
}

func TestMaxInstancesDropsWaitingForDoneTicket(t *testing.T) {
	dir, scope := setup(t, slotConfig)
	first, _ := tasks.Add(scope, "one", nil)
	second, _ := tasks.Add(scope, "two", nil)
	live := fakePanes{}
	var fired []string
	runFor(t, dir, live, &fired)
	if len(fired) != 1 || fired[0] != first.ID {
		t.Fatalf("fired %v", fired)
	}
	if err := tasks.Move(scope, second.ID, "done", ""); err != nil {
		t.Fatal(err)
	}
	delete(live, first.ID)
	runFor(t, dir, live, &fired)
	if len(fired) != 1 {
		t.Fatalf("waiting dispatch fired for a done ticket: %v", fired)
	}
}

// A pane table that cannot be read must hold the dispatch, never fire it over the limit.
func TestMaxInstancesWaitsWhenPanesUnreadable(t *testing.T) {
	dir, scope := setup(t, slotConfig)
	first, _ := tasks.Add(scope, "one", nil)
	var fired []string
	ctx, stop := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer stop()
	err := RunWith(ctx, dir, func(a Action) error {
		if !a.Close {
			fired = append(fired, a.Event.Task)
		}
		return nil
	}, func(string, string) (map[string]bool, error) { return nil, errTmux })
	if err != nil {
		t.Fatal(err)
	}
	if len(fired) != 0 {
		t.Fatalf("fired %v while panes were unreadable (ticket %s)", fired, first.ID)
	}
	events, _ := tasks.ReadEvents(scope, 0)
	audited := 0
	for _, ev := range events {
		if ev.Kind == "dispatch" && strings.Contains(ev.Message, "waiting: cannot read role planner panes") {
			audited++
		}
	}
	if audited != 1 {
		t.Fatalf("audited %d unreadable-pane waits, want 1", audited)
	}
}

var errTmux = errors.New("tmux server not running")
