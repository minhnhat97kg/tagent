package board

import (
	"strings"
	"tagent/internal/tasks"
	"testing"
)

const reviewLoopConfig = `roles: {builder: {agent: pi}, reviewer: {agent: pi}}
statuses:
 - {id: todo}
 - {id: build, role: builder}
 - {id: review, role: reviewer, next: [done, build, blocked], max_loops: 3}
 - {id: blocked}
 - {id: done}
max_fires_per_hour: 1
`

// runReviewLoop drives build -> review entries; QA rejects with findings on every
// run but the last, which gets verdict. It returns the actions of the last review entry.
func runReviewLoop(t *testing.T, verdict string, rejectRuns int) (tasks.Task, []Action) {
	t.Helper()
	dir, scope := setup(t, reviewLoopConfig)
	cfg, _ := tasks.LoadBoardConfig(dir)
	task, _ := tasks.Add(scope, "ticket", []string{"ignored"})
	builder := tasks.MutationOptions{Actor: "builder"}
	reviewer := tasks.MutationOptions{Actor: "reviewer"}
	if err := tasks.Move(scope, task.ID, "build", "", builder); err != nil {
		t.Fatal(err)
	}
	var actions []Action
	for run := 1; run <= rejectRuns+1; run++ {
		if err := tasks.Move(scope, task.ID, "review", "", builder); err != nil {
			t.Fatal(err)
		}
		events, _ := tasks.ReadEvents(scope, 0)
		var err error
		actions, err = Actions(dir, cfg, events[len(events)-1], task)
		if err != nil || len(actions) != 1 {
			t.Fatal(actions, err)
		}
		if reason := actions[0].BlockReason; reason != "" {
			t.Fatalf("review run %d blocked before QA ran: %s", run, reason)
		}
		if run <= rejectRuns {
			if err := tasks.Move(scope, task.ID, "build", "src/a.go:1 bad "+string(rune('0'+run)), reviewer); err != nil {
				t.Fatal(err)
			}
		}
	}
	return task, actions
}

// Review max_loops counts rejections (review -> build). After 3 rejections and a
// fix, the 4th review run is dispatched; when QA passes it, the ticket moves forward.
func TestReviewRunsOnceMoreAfterMaxLoopRejections(t *testing.T) {
	dir, scope := setup(t, reviewLoopConfig)
	cfg, _ := tasks.LoadBoardConfig(dir)
	task, _ := tasks.Add(scope, "ticket", []string{"ignored"})
	builder := tasks.MutationOptions{Actor: "builder"}
	reviewer := tasks.MutationOptions{Actor: "reviewer"}
	if err := tasks.Move(scope, task.ID, "build", "", builder); err != nil {
		t.Fatal(err)
	}
	for run := 1; run <= 4; run++ {
		if err := tasks.Move(scope, task.ID, "review", "", builder); err != nil {
			t.Fatal(err)
		}
		events, _ := tasks.ReadEvents(scope, 0)
		actions, err := Actions(dir, cfg, events[len(events)-1], task)
		if err != nil || len(actions) != 1 {
			t.Fatal(actions, err)
		}
		if reason := actions[0].BlockReason; reason != "" {
			t.Fatalf("review run %d blocked before QA ran: %s", run, reason)
		}
		if run == 4 {
			// Final allowed run passes: moves forward, not back to build.
			if err := tasks.Move(scope, task.ID, "done", "", reviewer); err != nil {
				t.Fatal(err)
			}
			events, _ = tasks.ReadEvents(scope, 0)
			actions, err = Actions(dir, cfg, events[len(events)-1], task)
			if err != nil {
				t.Fatal(err)
			}
			for _, a := range actions {
				if a.BlockReason != "" {
					t.Fatal("pass on final run was blocked", a.BlockReason)
				}
			}
			return
		}
		if err := tasks.Move(scope, task.ID, "build", "src/a.go:1 bad", reviewer); err != nil {
			t.Fatal(err)
		}
	}
}

// Final allowed run fails: the rejection past the cap blocks the ticket with the
// step, the cap and the last findings instead of handing back to build.
func TestReviewBlocksWhenFinalAllowedRunFails(t *testing.T) {
	dir, scope := setup(t, reviewLoopConfig)
	cfg, _ := tasks.LoadBoardConfig(dir)
	task, _ := tasks.Add(scope, "ticket", []string{"ignored"})
	builder := tasks.MutationOptions{Actor: "builder"}
	reviewer := tasks.MutationOptions{Actor: "reviewer"}
	if err := tasks.Move(scope, task.ID, "build", "", builder); err != nil {
		t.Fatal(err)
	}
	for run := 1; run <= 4; run++ {
		if err := tasks.Move(scope, task.ID, "review", "", builder); err != nil {
			t.Fatal(err)
		}
		if err := tasks.Move(scope, task.ID, "build", "src/a.go:9 nil deref", reviewer); err != nil {
			t.Fatal(err)
		}
	}
	task, _ = tasks.Get(scope, task.ID)
	events, _ := tasks.ReadEvents(scope, 0)
	actions, err := Actions(dir, cfg, events[len(events)-1], task)
	if err != nil || len(actions) != 1 {
		t.Fatal(actions, err)
	}
	reason := actions[0].BlockReason
	for _, want := range []string{"review rejected 4/3", "src/a.go:9 nil deref"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("block reason %q missing %q", reason, want)
		}
	}
}

const checkLoopConfig = `roles: {implementer: {agent: pi}}
statuses:
 - {id: todo}
 - {id: build, role: implementer}
 - {id: test, run: "go test ./...", pass: done, fail: build, max_loops: 3}
 - {id: blocked}
 - {id: done}
max_fires_per_hour: 1
`

// driveCheckLoop runs a run: step with max_loops 3 and on-failure build. Every run
// fails with findings except the last one when lastPasses. It returns the actions of
// the final event and the task as it stands then.
func driveCheckLoop(t *testing.T, lastPasses bool) ([]Action, tasks.Task) {
	t.Helper()
	dir, scope := setup(t, checkLoopConfig)
	cfg, _ := tasks.LoadBoardConfig(dir)
	task, _ := tasks.Add(scope, "ticket", []string{"ignored"})
	impl := tasks.MutationOptions{Actor: "implementer"}
	ci := tasks.MutationOptions{Actor: "ci"}
	if err := tasks.Move(scope, task.ID, "build", "", impl); err != nil {
		t.Fatal(err)
	}
	for run := 1; run <= 4; run++ {
		if err := tasks.Move(scope, task.ID, "test", "", impl); err != nil {
			t.Fatal(err)
		}
		task, _ = tasks.Get(scope, task.ID)
		events, _ := tasks.ReadEvents(scope, 0)
		actions, err := Actions(dir, cfg, events[len(events)-1], task)
		if err != nil || len(actions) == 0 {
			t.Fatal(actions, err)
		}
		if reason := actions[0].BlockReason; reason != "" {
			t.Fatalf("test run %d blocked before it ran: %s", run, reason)
		}
		if run == 4 && lastPasses {
			if err := tasks.Move(scope, task.ID, "done", "CI exit code 0", ci); err != nil {
				t.Fatal(err)
			}
			break
		}
		if err := tasks.Move(scope, task.ID, "build", "src/a.go:9 nil deref", ci); err != nil {
			t.Fatal(err)
		}
	}
	task, _ = tasks.Get(scope, task.ID)
	events, _ := tasks.ReadEvents(scope, 0)
	actions, err := Actions(dir, cfg, events[len(events)-1], task)
	if err != nil {
		t.Fatal(err)
	}
	return actions, task
}

// A run: step's final allowed run passes: the ticket moves forward, not back to build.
func TestRunCheckRunsOnceMoreAfterMaxLoopRejections(t *testing.T) {
	actions, task := driveCheckLoop(t, true)
	if task.Status != "done" {
		t.Fatal("final passing run did not move forward", task.Status)
	}
	for _, a := range actions {
		if a.BlockReason != "" {
			t.Fatal("pass on final run was blocked", a.BlockReason)
		}
	}
}

// A run: step's final allowed run fails: the rejection past the cap blocks the ticket
// with the step, the cap and the last findings.
func TestRunCheckBlocksWhenFinalAllowedRunFails(t *testing.T) {
	actions, _ := driveCheckLoop(t, false)
	if len(actions) != 1 {
		t.Fatal(actions)
	}
	reason := actions[0].BlockReason
	for _, want := range []string{"test rejected 4/3", "src/a.go:9 nil deref"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("block reason %q missing %q", reason, want)
		}
	}
}

// A retry on the review step re-dispatches the latest fix; it is not a rejection and
// must not block it before QA runs.
func TestRetryOnReviewDoesNotBlockLatestFix(t *testing.T) {
	dir, scope := setup(t, reviewLoopConfig)
	cfg, _ := tasks.LoadBoardConfig(dir)
	task, _ := tasks.Add(scope, "ticket", []string{"ignored"})
	builder := tasks.MutationOptions{Actor: "builder"}
	reviewer := tasks.MutationOptions{Actor: "reviewer"}
	if err := tasks.Move(scope, task.ID, "build", "", builder); err != nil {
		t.Fatal(err)
	}
	for run := 1; run <= 3; run++ {
		if err := tasks.Move(scope, task.ID, "review", "", builder); err != nil {
			t.Fatal(err)
		}
		if err := tasks.Move(scope, task.ID, "build", "src/a.go:1 bad", reviewer); err != nil {
			t.Fatal(err)
		}
	}
	if err := tasks.Move(scope, task.ID, "review", "", builder); err != nil {
		t.Fatal(err)
	}
	if err := tasks.Retry(scope, task.ID); err != nil {
		t.Fatal(err)
	}
	task, _ = tasks.Get(scope, task.ID)
	events, _ := tasks.ReadEvents(scope, 0)
	actions, err := Actions(dir, cfg, events[len(events)-1], task)
	if err != nil || len(actions) != 1 {
		t.Fatal(actions, err)
	}
	if reason := actions[0].BlockReason; reason != "" {
		t.Fatalf("retry of the latest review blocked before QA ran: %s", reason)
	}
}
