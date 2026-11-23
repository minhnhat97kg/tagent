package board

import (
	"os"
	"path/filepath"
	"testing"

	"tagent/internal/tasks"
)

const featureWorkflow = `name: feature
inputs:
  goal: {required: true}
jobs:
  plan: {status: planning, agent: planner}
  build: {needs: plan, status: building, agent: implementer}
  test: {needs: build, status: testing, run: go test ./...}
  review: {needs: test, status: reviewing, agent: reviewer}
`

func newFormatBoard(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "feature.yml"), []byte(featureWorkflow), 0600); err != nil {
		t.Fatal(err)
	}
	if err := tasks.SaveBoardConfig(dir, []byte("default_workflow: feature\n")); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateMapsOldStatusesToDefaultWorkflow(t *testing.T) {
	dir, scope := setup(t, jiraConfig)
	build, _ := tasks.Add(scope, "build ticket", nil)
	review, _ := tasks.Add(scope, "review ticket", nil)
	for id, status := range map[string]string{build.ID: "build", review.ID: "review"} {
		if err := tasks.Move(scope, id, status, ""); err != nil {
			t.Fatal(err)
		}
	}
	// A ticket still on the implicit board workflow (or with none at all)
	// migrates too; the old status id names the job.
	newFormatBoard(t, dir)

	migrations, err := Migrate(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 {
		t.Fatalf("migrations: %+v", migrations)
	}
	want := map[string]string{build.ID: "building", review.ID: "reviewing"}
	for _, m := range migrations {
		if m.Skipped != "" || m.Workflow != "feature" || m.To != want[m.ID] {
			t.Fatalf("mapping: %+v", m)
		}
		ticket, _ := tasks.Get(scope, m.ID)
		if ticket.Workflow != "feature" || ticket.Status != m.To || ticket.Inputs["goal"] == "" {
			t.Fatalf("ticket: %+v", ticket)
		}
	}

	// Idempotent: nothing left to migrate.
	again, err := Migrate(dir, false)
	if err != nil || len(again) != 0 {
		t.Fatalf("second run: %+v %v", again, err)
	}
}

func TestMigrateDryRunLeavesTicketsAlone(t *testing.T) {
	dir, scope := setup(t, jiraConfig)
	ticket, _ := tasks.Add(scope, "ticket", nil)
	if err := tasks.Move(scope, ticket.ID, "review", ""); err != nil {
		t.Fatal(err)
	}
	newFormatBoard(t, dir)
	migrations, err := Migrate(dir, true)
	if err != nil || len(migrations) != 1 || migrations[0].To != "reviewing" {
		t.Fatalf("dry run: %+v %v", migrations, err)
	}
	got, _ := tasks.Get(scope, ticket.ID)
	if got.Status != "review" || got.Workflow == "feature" {
		t.Fatalf("dry run changed the ticket: %+v", got)
	}
}
