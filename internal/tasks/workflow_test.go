package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkflowTicket(t *testing.T) {
	dir, scope := testBoard(t)
	path := filepath.Join(dir, ".tasks", "workflows")
	if e := os.MkdirAll(path, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(path, "sample.yml"), []byte("name: sample\ninputs:\n  goal: {required: true}\njobs:\n  collect: {run: echo ok}\n  review: {needs: collect, agent: reviewer, max-loops: 1, on-failure: collect}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Add(scope, "missing", nil, MutationOptions{Workflow: "sample"}); e == nil {
		t.Fatal("required input accepted")
	}
	ticket, e := Add(scope, "ticket", nil, MutationOptions{Workflow: "sample", Inputs: map[string]string{"goal": "test"}})
	if e != nil {
		t.Fatal(e)
	}
	cfg, e := configForTaskScope(scope, ticket)
	if e != nil || len(cfg.Statuses) != 5 {
		t.Fatal(cfg, e)
	}
	zero := 0
	converted, err := ConfigForTask(dir, BoardConfig{Statuses: []Status{{ID: "review", FreshAfter: &zero}}}, ticket)
	if err != nil || converted.Statuses[2].FreshAfter == nil || *converted.Statuses[2].FreshAfter != 0 {
		t.Fatalf("workflow override lost: %+v %v", converted, err)
	}
	if e = Move(scope, ticket.ID, "review", ""); e == nil {
		t.Fatal("skipped first job")
	}
	if e = Move(scope, ticket.ID, "collect", ""); e != nil {
		t.Fatal(e)
	}
	events, _ := ReadEvents(scope, 0)
	seq := events[len(events)-1].Seq
	if e = FinishEntry(scope, ticket.ID, "review", "CI", seq, false, MutationOptions{Actor: "ci", Outputs: map[string]string{"file": "data"}}); e != nil {
		t.Fatal(e)
	}
	got, _ := Get(scope, ticket.ID)
	if got.Outputs["collect"]["file"] != "data" {
		t.Fatal(got)
	}
	if e = FinishEntry(scope, ticket.ID, "review", "stale", seq, false, MutationOptions{Actor: "ci", Outputs: map[string]string{"file": "wrong"}}); e != nil {
		t.Fatal(e)
	}
	got, _ = Get(scope, ticket.ID)
	if got.Outputs["collect"]["file"] != "data" {
		t.Fatal("stale outputs published")
	}
	if e = Move(scope, ticket.ID, "collect", "retry"); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(path, "sample.yml")); e != nil {
		t.Fatal(e)
	}
	if e = Move(scope, ticket.ID, "done", ""); e == nil {
		t.Fatal("missing workflow silently accepted")
	}
}
