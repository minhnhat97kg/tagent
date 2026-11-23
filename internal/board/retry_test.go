package board

import (
	"context"
	"errors"
	"strings"
	"tagent/internal/tasks"
	"testing"
	"time"
)

func TestRetryDispatchAndCaps(t *testing.T) {
	dir, scope := setup(t, "statuses:\n  - {id: todo}\n  - {id: doing, role: builder, max_loops: 2}\n  - {id: blocked}\n  - {id: done}\n")
	ticket, err := tasks.Add(scope, "work", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = tasks.Move(scope, ticket.ID, "doing", ""); err != nil {
		t.Fatal(err)
	}
	if err = tasks.Retry(scope, ticket.ID); err != nil {
		t.Fatal(err)
	}
	if err = tasks.Retry(scope, ticket.ID); err != nil {
		t.Fatal(err)
	}
	calls := 0
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := Run(ctx, dir, func(a Action) error { calls++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	run()
	run()
	got, _ := tasks.Get(scope, ticket.ID)
	if calls != 2 || got.Status != "blocked" {
		t.Fatal(calls, got)
	}
}

func TestAsyncFailureKeepsOriginalIdentity(t *testing.T) {
	dir, scope := setup(t, "triggers: [{on: create, agent: builder}]")
	a, _ := tasks.Add(scope, "first", nil)
	b, _ := tasks.Add(scope, "second", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	var first Action
	if err := Run(ctx, dir, func(action Action) error {
		if action.Event.Task == a.ID {
			first = action
		} else {
			first.OnError(errors.New("late failure"))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	events, _ := tasks.ReadEvents(scope, 0)
	found := false
	for _, ev := range events {
		if !strings.Contains(ev.Message, "async failure: late failure") {
			continue
		}
		if ev.Task == b.ID {
			t.Fatal("misattributed", ev)
		}
		found = ev.Task == a.ID
	}
	if !found {
		t.Fatal("missing async audit")
	}
}
