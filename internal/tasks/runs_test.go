package tasks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// runSteps mirrors what ConfigForTask builds for a workflow: roles, a run step
// with pass/fail, and a failure route back to an earlier step.
func runSteps() BoardConfig {
	return BoardConfig{Statuses: []Status{
		{ID: "todo", Next: []string{"planning"}},
		{ID: "planning", Role: "planner", Job: "plan", Next: []string{"build", "blocked"}},
		{ID: "build", Role: "implementer", Job: "build", Next: []string{"review", "blocked"}, Failure: "blocked"},
		{ID: "review", Role: "reviewer", Job: "review", Next: []string{"done", "build", "blocked"}, Failure: "build"},
		{ID: "test", Run: "go test ./...", Job: "test", Pass: "review", Fail: "build", Failure: "build", Next: []string{"review", "build", "blocked"}},
		{ID: "blocked"},
		{ID: "done", Next: []string{}},
	}}
}

var runStart = time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

// ev numbers events by minute so durations read as whole minutes.
func ev(seq int64, kind, from, to, commentID string) Event {
	return Event{Seq: seq, Task: "22", Kind: kind, From: from, To: to, CommentID: commentID, Time: runStart.Add(time.Duration(seq) * time.Minute)}
}

func runTask(comments ...TaskComment) Task {
	return Task{ID: "22", Title: "Loop ticket", Status: "build", Comments: comments}
}

func TestBuildRunsLoop(t *testing.T) {
	events := []Event{
		ev(1, "status", "todo", "planning", ""),
		{Seq: 2, Task: "22", Kind: "dispatch", To: "planning", Role: "planner", Harness: "pi", Model: "gpt", Pane: "%1", PaneName: "plan-22", Prompt: "plan it", Time: runStart.Add(2 * time.Minute)},
		ev(3, "status", "planning", "build", "c1"),
		ev(4, "status", "build", "review", "c2"),
		ev(5, "status", "review", "build", "c3"),
		ev(6, "status", "build", "review", "c4"),
		ev(7, "status", "review", "done", "c5"),
	}
	task := runTask(
		TaskComment{ID: "c1", Author: "planner", Body: "plan ready"},
		TaskComment{ID: "c2", Author: "implementer", Body: "built it"},
		TaskComment{ID: "c3", Author: "reviewer", Body: "changes: fix X"},
		TaskComment{ID: "c4", Author: "implementer", Body: "fixed X"},
		TaskComment{ID: "c5", Author: "reviewer", Body: "looks good"},
	)
	runs := BuildRuns(task, runSteps(), events, runStart.Add(time.Hour))
	want := []struct {
		step   string
		loop   int
		result string
	}{
		{"planning", 1, "pass"},
		{"build", 1, "pass"},
		{"review", 1, "changes"},
		{"build", 2, "pass"},
		{"review", 2, "pass"},
	}
	if len(runs) != len(want) {
		t.Fatalf("got %d entries: %+v", len(runs), runs)
	}
	for i, w := range want {
		if runs[i].Step != w.step || runs[i].Loop != w.loop || runs[i].Result != w.result {
			t.Fatalf("entry %d: %+v", i, runs[i])
		}
	}
	if runs[1].Comment != "built it" || runs[1].Duration != time.Minute {
		t.Fatalf("handoff: %+v", runs[1])
	}
	if runs[3].Reason != "changes: fix X" {
		t.Fatalf("loop reason: %+v", runs[3])
	}
	if r := runs[0]; r.Role != "planner" || r.Harness != "pi" || r.Model != "gpt" || r.Pane != "%1" || r.Prompt != "plan it" {
		t.Fatalf("dispatch details: %+v", r)
	}
}

func TestBuildRunsBlockedAndResume(t *testing.T) {
	events := []Event{
		ev(1, "status", "todo", "build", ""),
		ev(2, "status", "build", "blocked", "c1"),
		ev(3, "comment", "", "", "c2"),
		ev(4, "status", "blocked", "build", "c2"),
	}
	task := runTask(
		TaskComment{ID: "c1", Author: "implementer", Body: "which API?"},
		TaskComment{ID: "c2", Author: "user", Body: "use v2"},
	)
	runs := BuildRuns(task, runSteps(), events, runStart.Add(10*time.Minute))
	if len(runs) != 2 {
		t.Fatalf("got %d entries: %+v", len(runs), runs)
	}
	if runs[0].Result != "blocked" || runs[0].Comment != "which API?" || runs[0].Loop != 1 {
		t.Fatalf("blocked entry: %+v", runs[0])
	}
	if runs[1].Loop != 2 || runs[1].Reason != "use v2" || runs[1].Result != "current" || runs[1].End != nil {
		t.Fatalf("resumed entry: %+v", runs[1])
	}
	if runs[1].Duration != 6*time.Minute {
		t.Fatalf("current duration: %v", runs[1].Duration)
	}
}

func TestBuildRunsCI(t *testing.T) {
	pass := runTask(TaskComment{ID: "c1", Author: "ci", Body: "CI exit code 0\nok tagent", Attachments: []Attachment{{Name: "ci.log", File: "log"}}})
	events := []Event{
		ev(1, "status", "todo", "test", ""),
		{Seq: 2, Task: "22", Kind: "dispatch", To: "test", Role: "ci", Prompt: "go test ./...", Time: runStart.Add(2 * time.Minute)},
		{Seq: 3, Task: "22", Kind: "status", From: "test", To: "review", CommentID: "c1", Time: runStart.Add(4 * time.Minute), Outputs: map[string]string{"plan": ".tasks/plans/26.md"}},
	}
	runs := BuildRuns(pass, runSteps(), events, runStart.Add(time.Hour))
	// The review entry opens too; only the test entry ended.
	if len(runs) != 2 || runs[0].Step != "test" || runs[0].Result != "pass" || runs[0].ExitCode == nil || *runs[0].ExitCode != 0 {
		t.Fatalf("ci pass: %+v", runs)
	}
	if runs[0].Outputs["plan"] != ".tasks/plans/26.md" || len(runs[0].Attachments) != 1 || runs[0].Role != "ci" {
		t.Fatalf("ci details: %+v", runs[0])
	}

	fail := runTask(TaskComment{ID: "c1", Author: "ci", Body: "CI exit code 1\n--- FAIL: TestX"})
	events = []Event{
		ev(1, "status", "todo", "test", ""),
		ev(2, "status", "test", "build", "c1"),
	}
	runs = BuildRuns(fail, runSteps(), events, runStart.Add(time.Hour))
	if len(runs) != 2 || runs[1].Step != "build" || runs[0].Result != "fail" || runs[0].ExitCode == nil || *runs[0].ExitCode != 1 {
		t.Fatalf("ci fail: %+v", runs)
	}
}

func TestBuildRunsRetry(t *testing.T) {
	events := []Event{
		ev(1, "status", "todo", "build", ""),
		ev(2, "retry", "", "build", ""),
	}
	runs := BuildRuns(runTask(), runSteps(), events, runStart.Add(3*time.Minute))
	if len(runs) != 2 || runs[0].Result != "retry" || runs[1].Loop != 2 || runs[1].Result != "current" {
		t.Fatalf("retry: %+v", runs)
	}
}

func TestBuildRunsIgnoresStaleResult(t *testing.T) {
	events := []Event{
		ev(1, "status", "todo", "test", ""),
		ev(2, "comment", "", "", "c1"), // stale: no status event, so no entry
	}
	task := runTask(TaskComment{ID: "c1", Author: "ci", Body: "Stale result (ticket moved): CI exit code 1"})
	runs := BuildRuns(task, runSteps(), events, runStart.Add(time.Minute))
	if len(runs) != 1 || runs[0].Result != "current" || runs[0].ExitCode != nil || runs[0].Comment != "" {
		t.Fatalf("stale: %+v", runs)
	}
}

func TestBuildRunsOldEntriesAndCommentOutputs(t *testing.T) {
	// No dispatch events: rows still carry the status moves and results.
	events := []Event{
		ev(1, "status", "todo", "build", ""),
		{Seq: 2, Task: "22", Kind: "comment", CommentID: "c1", Time: runStart.Add(2 * time.Minute), Outputs: map[string]string{"plan": "x"}},
		ev(3, "status", "build", "review", "c1"),
	}
	task := runTask(TaskComment{ID: "c1", Author: "implementer", Body: "done"})
	runs := BuildRuns(task, runSteps(), events, runStart.Add(time.Hour))
	if len(runs) != 2 || runs[0].Step != "build" || runs[0].Result != "pass" || runs[0].Role != "" || runs[0].Prompt != "" {
		t.Fatalf("old entry: %+v", runs)
	}
	if runs[0].Outputs["plan"] != "x" {
		t.Fatalf("comment outputs: %+v", runs[0])
	}
}

func TestRecordDispatchAndReadEvents(t *testing.T) {
	_, scope := testBoard(t)
	task, err := Add(scope, "Run details", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = RecordDispatch(scope, task.ID, Event{To: "build", Role: "implementer", Harness: "pi", Model: "gpt", Pane: "%12", PaneName: "impl-22", Prompt: "build it", Outputs: map[string]string{"plan": "p"}})
	if err != nil {
		t.Fatal(err)
	}
	events, err := ReadEvents(scope, 0)
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if last.Kind != "dispatch" || last.Actor != "system" || last.To != "build" || last.Pane != "%12" || last.Harness != "pi" || last.Prompt != "build it" || last.Outputs["plan"] != "p" {
		t.Fatalf("dispatch event: %+v", last)
	}
	// A line written before these fields existed still parses.
	path := filepath.Join(BoardDir(scope), "events.jsonl")
	if err = os.WriteFile(path, []byte(`{"seq":1,"time":"2024-01-01T00:00:00Z","task":"1","kind":"status","actor":"user","from":"todo","to":"plan"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	events, err = ReadEvents(scope, 0)
	if err != nil || len(events) != 1 || events[0].From != "todo" || events[0].Prompt != "" {
		t.Fatalf("old line: %+v %v", events, err)
	}
	var e Event
	b, _ := json.Marshal(Event{Pane: "%1"})
	if err = json.Unmarshal(b, &e); err != nil || e.Pane != "%1" {
		t.Fatal(e, err)
	}
}

func TestMoveStoresEntryOutputs(t *testing.T) {
	dir, scope := testBoard(t)
	path := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "sample.yml"), []byte("name: sample\njobs:\n  collect: {run: echo ok, outputs: [plan]}\n  review: {needs: collect, agent: reviewer}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ticket, err := Add(scope, "ticket", nil, MutationOptions{Workflow: "sample"})
	if err != nil {
		t.Fatal(err)
	}
	if err = Move(scope, ticket.ID, "collect", "start"); err != nil {
		t.Fatal(err)
	}
	events, _ := ReadEvents(scope, 0)
	seq := events[len(events)-1].Seq
	if err = FinishEntry(scope, ticket.ID, "review", "CI exit code 0", seq, false, MutationOptions{Actor: "ci", Outputs: map[string]string{"plan": "plan.md"}}); err != nil {
		t.Fatal(err)
	}
	events, _ = ReadEvents(scope, 0)
	last := events[len(events)-1]
	if last.Kind != "status" || last.Outputs["plan"] != "plan.md" {
		t.Fatalf("status outputs: %+v", last)
	}
	// A comment-only move keeps its outputs on the comment event.
	if err = Move(scope, ticket.ID, ticket2Status(t, scope, ticket.ID), "note", MutationOptions{Actor: "ci", Outputs: map[string]string{"plan": "later.md"}}); err != nil {
		t.Fatal(err)
	}
	events, _ = ReadEvents(scope, 0)
	last = events[len(events)-1]
	if last.Kind != "comment" || last.Outputs["plan"] != "later.md" {
		t.Fatalf("comment outputs: %+v", last)
	}
}

func ticket2Status(t *testing.T, scope, id string) string {
	t.Helper()
	task, err := Get(scope, id)
	if err != nil {
		t.Fatal(err)
	}
	return task.Status
}

func TestBuildRunsSkipsAuditLines(t *testing.T) {
	events := []Event{
		ev(1, "status", "todo", "build", ""),
		{Seq: 2, Task: "22", Kind: "dispatch", Message: "implementer: reserved; outcome uncertain", Time: runStart.Add(2 * time.Minute)},
		{Seq: 3, Task: "22", Kind: "dispatch", To: "build", Role: "implementer", Pane: "%7", Time: runStart.Add(2 * time.Minute)},
	}
	runs := BuildRuns(runTask(), runSteps(), events, runStart)
	if len(runs) != 1 || runs[0].Role != "implementer" || runs[0].Pane != "%7" {
		t.Fatalf("audit line took the detail slot: %+v", runs)
	}
}

func TestBuildRunsCarriesTierAndRoute(t *testing.T) {
	events := []Event{
		ev(1, "status", "todo", "planning", ""),
		{Seq: 2, Task: "22", Kind: "dispatch", To: "planning", Role: "planner", Harness: "pi", Model: "gpt-6-luna", Tier: "fast", Route: "tier fast (Claude 5h 93%)", Time: runStart.Add(2 * time.Minute)},
	}
	runs := BuildRuns(runTask(), runSteps(), events, runStart.Add(time.Hour))
	if runs[0].Tier != "fast" || runs[0].Route != "tier fast (Claude 5h 93%)" {
		t.Fatalf("%+v", runs[0])
	}
}
