package tasks

import (
	"os"
	"path/filepath"
	"testing"
)

func defaultWorkflowBoard(t *testing.T) (string, string) {
	t.Helper()
	dir, scope := testBoard(t)
	path := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	wf := "name: feature\ninputs:\n  goal: {required: true}\njobs:\n  plan: {status: planning, agent: planner}\n  build: {needs: plan, status: building, agent: implementer}\n  test: {needs: build, status: testing, run: go test ./...}\n"
	if err := os.WriteFile(filepath.Join(path, "feature.yml"), []byte(wf), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveBoardConfig(dir, []byte("roles: {planner: {agent: pi}, implementer: {agent: pi}}\ndefault_workflow: feature\n")); err != nil {
		t.Fatal(err)
	}
	return dir, scope
}

func TestDefaultWorkflowAttachesOnMoveOutOfTodo(t *testing.T) {
	_, scope := defaultWorkflowBoard(t)
	ticket, err := Add(scope, "Goal text", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Move(scope, ticket.ID, "planning", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := Get(scope, ticket.ID)
	if got.Workflow != "feature" || got.Status != "planning" || got.Inputs["goal"] != "Goal text" {
		t.Fatalf("attached: %+v", got)
	}
}

func TestMoveToJobIDLandsOnItsStatus(t *testing.T) {
	_, scope := defaultWorkflowBoard(t)
	ticket, _ := Add(scope, "Goal text", nil)
	if err := Move(scope, ticket.ID, "plan", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := Get(scope, ticket.ID)
	if got.Workflow != "feature" || got.Status != "planning" {
		t.Fatalf("job id mapping: %+v", got)
	}
	// Once attached, job ids keep working as aliases of their statuses.
	if _, err := Update(scope, ticket.ID, "build", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = Get(scope, ticket.ID)
	if got.Status != "building" {
		t.Fatalf("update job id: %+v", got)
	}
}

func TestMoveToBlockedDoesNotAttach(t *testing.T) {
	_, scope := defaultWorkflowBoard(t)
	ticket, _ := Add(scope, "Goal text", nil)
	if err := Move(scope, ticket.ID, "blocked", "question"); err != nil {
		t.Fatal(err)
	}
	got, _ := Get(scope, ticket.ID)
	if got.Workflow != "" || got.Status != "blocked" {
		t.Fatalf("blocked ticket got a workflow: %+v", got)
	}
}

func TestNoDefaultWorkflowRejectsLeavingTodo(t *testing.T) {
	_, scope := testBoard(t)
	ticket, _ := Add(scope, "Goal text", nil)
	if err := Move(scope, ticket.ID, "plan", ""); err == nil {
		t.Fatal("unknown status accepted without a workflow")
	}
	got, _ := Get(scope, ticket.ID)
	if got.Workflow != "" || got.Status != "todo" {
		t.Fatalf("ticket changed: %+v", got)
	}
}
