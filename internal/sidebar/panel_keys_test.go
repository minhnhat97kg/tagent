package sidebar

import (
	"tagent/internal/tasks"
	"testing"
)

func TestPanelUsesBoardActions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, key := range []string{"n", "s", "enter", "H", "L", "e", "C", "x"} {
		t.Run(key, func(t *testing.T) {
			panel := boardTestModel()
			panel.repoDir = t.TempDir()
			panel.boardMode = false
			panel = panel.boardContext()
			board := panel
			board.boardMode = true
			p, pc := boardPress(panel, key)
			b, bc := boardPress(board, key)
			if p.taskForm != b.taskForm || p.taskStep != b.taskStep || p.boardDrawer != b.boardDrawer || p.taskDelete != b.taskDelete || (pc == nil) != (bc == nil) {
				t.Fatalf("panel and board differ for %s", key)
			}
			if key == "H" || key == "L" {
				scope := tasks.Scope(panel.repoDir)
				task, err := tasks.Add(scope, "ticket", nil)
				if err != nil {
					t.Fatal(err)
				}
				panel.notesScope, panel.notes = scope, []tasks.Task{task}
				panel.taskCursor = 0
				moved, cmd := boardPress(panel, key)
				_ = moved
				if cmd != nil {
					cmd()
				}
				got := tasks.List(scope)
				want := "todo"
				if key == "L" {
					want = "doing"
				}
				if len(got) != 1 || got[0].Status != want {
					t.Fatalf("move: %v", got)
				}
			}
		})
	}
}
