package board

import (
	"context"
	"strings"
	"tagent/internal/tasks"
	"testing"
	"time"
)

const jiraConfig = `roles: {implementer: {agent: pi, model: chosen}}
statuses:
 - {id: todo}
 - {id: build, role: implementer, max_loops: 2}
 - {id: review}
 - {id: blocked}
 - {id: done}
max_fires_per_hour: 1
`

// handBackConfig is jiraConfig with review handing the ticket back to build.
var handBackConfig = strings.Replace(jiraConfig, "- {id: review}", "- {id: review, next: [build, blocked, done]}", 1)

func TestStatusEntryReentryAndLoopLimit(t *testing.T) {
	dir, scope := setup(t, handBackConfig)
	cfg, _ := tasks.LoadBoardConfig(dir)
	task, _ := tasks.Add(scope, "ticket", []string{"ignored"})
	if err := tasks.Move(scope, task.ID, "build", "", tasks.MutationOptions{Actor: "implementer"}); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Move(scope, task.ID, "review", ""); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		// QA hands back to build: hand-back i of max_loops 2.
		if err := tasks.Move(scope, task.ID, "build", "findings", tasks.MutationOptions{Actor: "qa"}); err != nil {
			t.Fatal(err)
		}
		events, _ := tasks.ReadEvents(scope, 0)
		ev := events[len(events)-1]
		task, _ = tasks.Get(scope, task.ID)
		actions, err := Actions(dir, cfg, ev, task)
		if err != nil || len(actions) != 1 {
			t.Fatal(actions, err)
		}
		a := actions[0]
		if i <= 2 {
			if a.Agent != "implementer" || a.Role != "pi" || a.Model != "chosen" || a.BlockReason != "" {
				t.Fatal(a)
			}
		} else if a.BlockReason == "" {
			t.Fatal("loop limit missed")
		}
		if i < 3 {
			tasks.Move(scope, task.ID, "review", "")
		}
	}
	// A capped role fire cannot prevent the third entry from safety blocking.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	calls := 0
	if err := Run(ctx, dir, func(Action) error { calls++; return nil }); err != nil {
		t.Fatal(err)
	}
	got, _ := tasks.Get(scope, task.ID)
	if calls != 1 || got.Status != "blocked" {
		t.Fatal(calls, got)
	}
	found := false
	for _, c := range got.Comments {
		if c.Author == "system" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing safety explanation")
	}
	// Restart must not fire a reserved entry.
	ctx2, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	if err := Run(ctx2, dir, func(Action) error { t.Fatal("replayed entry"); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestCIEntryAndNonEntry(t *testing.T) {
	dir, _ := setup(t, jiraConfig)
	cfg, _ := tasks.LoadBoardConfig(dir)
	for _, actor := range []string{"ci", "implementer"} {
		a, err := Actions(dir, cfg, tasks.Event{Kind: "status", From: "review", To: "build", Actor: actor}, tasks.Task{ID: "abc"})
		if err != nil || len(a) != 1 {
			t.Fatal(actor, a, err)
		}
	}
	for _, ev := range []tasks.Event{{Kind: "create"}, {Kind: "assign"}, {Kind: "status", From: "build", To: "build"}, {Kind: "status", From: "todo", To: "build", Actor: "system"}} {
		a, err := Actions(dir, cfg, ev, tasks.Task{ID: "abc"})
		if err != nil || len(a) != 0 {
			t.Fatal(a, err)
		}
	}
}
