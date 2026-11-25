package tasks

import (
	"strings"
	"testing"
)

func TestCloseFromAnyStatusAndReopen(t *testing.T) {
	_, scope := testBoard(t)
	a, _ := Add(scope, "A", nil)
	b, _ := Add(scope, "B", nil)
	if err := SetNeeds(scope, b.ID, []string{a.ID}); err != nil { // open dependency: close still allowed
		t.Fatal(err)
	}
	if err := Move(scope, b.ID, "closed", "dropped"); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(scope, a.ID, "closed", nil); err != nil {
		t.Fatal(err)
	}
	if err := Move(scope, b.ID, "blocked", ""); err == nil || !strings.Contains(err.Error(), "reopen it with task set") {
		t.Fatalf("closed ticket left to blocked: %v", err)
	}
	if _, err := Update(scope, b.ID, "done", nil); err == nil {
		t.Fatal("closed ticket moved to done")
	}
	if err := Move(scope, b.ID, "todo", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := Get(scope, b.ID); got.Status != "todo" {
		t.Fatalf("reopen: %+v", got)
	}
}

func TestClosedDependencyKeepsDependentWaiting(t *testing.T) {
	_, scope := testBoard(t)
	a, _ := Add(scope, "A", nil)
	b, _ := Add(scope, "B", nil)
	if err := SetNeeds(scope, b.ID, []string{a.ID}); err != nil {
		t.Fatal(err)
	}
	if err := Move(scope, a.ID, "closed", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := Get(scope, b.ID)
	if reason := WaitingReason(scope, got); reason != "waiting for closed #"+a.ID {
		t.Fatalf("reason %q", reason)
	}
	if err := Move(scope, b.ID, "planning", ""); err == nil {
		t.Fatal("dependent left todo while its dependency is closed")
	}
}

func TestClosedIsNotAStatusWithoutWorkflow(t *testing.T) {
	dir, scope := defaultWorkflowBoard(t)
	_ = dir
	a, _ := Add(scope, "Goal", nil)
	if err := Move(scope, a.ID, "closed", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := Get(scope, a.ID); got.Workflow != "" || got.Status != "closed" {
		t.Fatalf("closing attached a workflow: %+v", got)
	}
}

func TestBoardYAMLCannotDeclareClosedLane(t *testing.T) {
	if _, err := ParseBoardConfig([]byte("statuses:\n  - {id: todo}\n  - {id: done}\nlanes: {closed: [todo]}\n")); err == nil {
		t.Fatal("closed lane accepted")
	}
}

func TestCloseFromBlockedBacklogAndDone(t *testing.T) {
	dir, scope := testBoard(t)
	if err := SaveBoardConfig(dir, []byte("lanes:\n  todo: [backlog, todo]\n  in progress: [review]\n  done: [done]\n")); err != nil {
		t.Fatal(err)
	}
	blocked, _ := Add(scope, "blocked one", nil)
	if err := Move(scope, blocked.ID, "blocked", "question"); err != nil {
		t.Fatal(err)
	}
	backlog, _ := Add(scope, "backlog one", nil)
	if err := Move(scope, backlog.ID, "backlog", ""); err != nil {
		t.Fatal(err)
	}
	done, _ := Add(scope, "done one", nil)
	if err := Move(scope, done.ID, "done", ""); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{blocked.ID, backlog.ID, done.ID} {
		if err := Move(scope, id, "closed", "dropped"); err != nil {
			t.Fatalf("close #%s: %v", id, err)
		}
		if got, _ := Get(scope, id); got.Status != "closed" {
			t.Fatalf("#%s status %q", id, got.Status)
		}
	}
}

func TestCloseDoneTicketWithWorkflow(t *testing.T) {
	_, scope := defaultWorkflowBoard(t)
	ticket, _ := Add(scope, "Goal text", nil)
	if err := SetWorkflow(scope, ticket.ID, "feature"); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(scope, ticket.ID, "done", nil); err != nil {
		t.Fatal(err)
	}
	if err := Move(scope, ticket.ID, "closed", "dropped"); err != nil {
		t.Fatalf("close done ticket with workflow: %v", err)
	}
	if got, _ := Get(scope, ticket.ID); got.Status != "closed" {
		t.Fatalf("status %q", got.Status)
	}
}
