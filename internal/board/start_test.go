package board

import (
	"testing"

	"tagent/internal/tasks"
)

func TestStartUsesDefaultWorkflow(t *testing.T) {
	dir, scope := setup(t, "columns: [todo, done]")
	newFormatBoard(t, dir)
	ticket, err := tasks.Add(scope, "goal text", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = Start(dir, ticket.ID, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := tasks.Get(scope, ticket.ID)
	if got.Workflow != "feature" || got.Status != "planning" || got.Inputs["goal"] != "goal text" {
		t.Fatalf("start: %+v", got)
	}
	// Starting again with no name reuses the ticket's workflow.
	if _, err = tasks.Update(scope, ticket.ID, "todo", nil); err != nil {
		t.Fatal(err)
	}
	if err = Start(dir, ticket.ID, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = tasks.Get(scope, ticket.ID)
	if got.Workflow != "feature" || got.Status != "planning" {
		t.Fatalf("restart: %+v", got)
	}
}

func TestStartWithoutDefaultWorkflowFails(t *testing.T) {
	dir, scope := setup(t, "columns: [todo, done]")
	ticket, _ := tasks.Add(scope, "goal text", nil)
	if err := Start(dir, ticket.ID, ""); err == nil {
		t.Fatal("start without a default workflow succeeded")
	}
	got, _ := tasks.Get(scope, ticket.ID)
	if got.Status != "todo" || got.Workflow != "" {
		t.Fatalf("ticket changed: %+v", got)
	}
}
