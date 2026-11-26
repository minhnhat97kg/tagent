package sidebar

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tagent/internal/tasks"
)

// Drawing the board runs once per pane and per frame. A ticket that names a
// workflow must not make every draw re-read and re-parse the workflow files
// (tasks.ConfigForTask -> workflow.List), so 60 such cards should allocate
// about as much as 60 plain cards.
func TestBoardRenderDoesNotReparseWorkflowPerCard(t *testing.T) {
	dir := t.TempDir()
	wf := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(wf, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"feature", "review", "docs"} {
		body := "name: " + name + "\njobs:\n  build: {agent: implementer}\n"
		if err := os.WriteFile(filepath.Join(wf, name+".yml"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	build := func(workflow string) Model {
		m := boardTestModel()
		m.boardDir, m.boardConfig = dir, cfg
		m.notes = nil
		for i := 0; i < 60; i++ {
			m.notes = append(m.notes, tasks.Task{
				ID: fmt.Sprint("t", i), Title: fmt.Sprintf("Ticket %d with a fairly long title", i),
				Text: "body", Status: "in progress", Workflow: workflow,
				Updated: time.Unix(int64(1000+i), 0),
			})
		}
		return m
	}
	plain, flow := build(""), build("feature")
	base := testing.AllocsPerRun(3, func() { _ = plain.View() })
	withWorkflow := testing.AllocsPerRun(3, func() { _ = flow.View() })
	// ponytail: each card still stats its workflow dirs and files (about 2.6x today, edits show up at once); resolve once per frame to go lower.
	if withWorkflow > 3*base {
		t.Fatalf("drawing 60 workflow tickets allocates %.0f, plain tickets %.0f: workflow files are re-parsed per card and per frame", withWorkflow, base)
	}
}
