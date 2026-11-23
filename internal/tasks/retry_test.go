package tasks

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRetryFreshEntryAndStaleCI(t *testing.T) {
	dir, scope := testBoard(t)
	path := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "retry.yml"), []byte("name: retry\njobs:\n  build: {run: echo ok}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ticket, err := Add(scope, "ticket", nil, MutationOptions{Workflow: "retry"})
	if err != nil {
		t.Fatal(err)
	}
	if err = Retry(scope, ticket.ID); err == nil {
		t.Fatal("role-less retry")
	}
	if err = Retry(scope, "missing"); err == nil {
		t.Fatal("missing retry")
	}
	if err = Move(scope, ticket.ID, "build", ""); err != nil {
		t.Fatal(err)
	}
	before, _ := Get(scope, ticket.ID)
	events, _ := ReadEvents(scope, 0)
	old := events[len(events)-1].Seq
	if err = Retry(scope, ticket.ID, MutationOptions{Actor: "operator"}); err != nil {
		t.Fatal(err)
	}
	after, _ := Get(scope, ticket.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("retry changed ticket")
	}
	events, _ = ReadEvents(scope, old)
	if len(events) != 1 || events[0].Kind != "retry" || events[0].To != "build" || events[0].Actor != "operator" {
		t.Fatal(events)
	}
	if err = FinishEntry(scope, ticket.ID, "done", "old CI", old, false, MutationOptions{Outputs: map[string]string{"bad": "value"}}); err != nil {
		t.Fatal(err)
	}
	after, _ = Get(scope, ticket.ID)
	if after.Status != "build" || len(after.Outputs) != 0 {
		t.Fatal("stale CI overwrote retry", after)
	}
	if err = FinishEntry(scope, ticket.ID, "done", "new CI", events[0].Seq, false); err != nil {
		t.Fatal(err)
	}
	if err = Retry(scope, ticket.ID); err == nil {
		t.Fatal("done retry")
	}
}
