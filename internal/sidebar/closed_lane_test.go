package sidebar

import (
	"testing"

	"tagent/internal/tasks"
)

func TestClosedTicketLandsInClosedLane(t *testing.T) {
	m := dashSample()
	m.boardConfig = tasks.BoardConfig{}
	m.notes = append(m.notes, tasks.Task{ID: "9", Title: "Dropped", Status: "closed"})
	if got := m.laneCards("closed"); len(got) != 1 || m.notes[got[0]].ID != "9" {
		t.Fatalf("closed lane: %v", got)
	}
	for _, i := range m.laneCards("in progress") {
		if m.notes[i].ID == "9" {
			t.Fatal("closed ticket in progress")
		}
	}
}

// H/L on a closed ticket must not reopen it: closed is not a column to step to.
func TestClosedTicketIsNotMovedByHL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	panel := boardTestModel()
	panel.repoDir = t.TempDir()
	panel.boardMode = false
	panel = panel.boardContext()
	scope := tasks.Scope(panel.repoDir)
	task, err := tasks.Add(scope, "dropped", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Move(scope, task.ID, "closed", ""); err != nil {
		t.Fatal(err)
	}
	panel.notesScope, panel.notes = scope, []tasks.Task{{ID: task.ID, Title: task.Title, Status: "closed"}}
	panel.taskCursor = 0
	for _, key := range []string{"L", "H"} {
		_, cmd := boardPress(panel, key)
		if cmd != nil {
			cmd()
		}
	}
	if got := tasks.List(scope); len(got) != 1 || got[0].Status != "closed" {
		t.Fatalf("closed ticket moved: %v", got)
	}
}
