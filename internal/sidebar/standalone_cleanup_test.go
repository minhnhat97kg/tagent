package sidebar

import (
	"testing"

	"tagent/internal/tasks"
)

func TestIsShellCommand(t *testing.T) {
	for cmd, want := range map[string]bool{"zsh": true, "-bash": true, "fish": true, "claude": false, "node": false, "pi": false, "lazygit": false, "2.1.330": false} {
		if isShellCommand(cmd) != want {
			t.Errorf("%q: want %v", cmd, want)
		}
	}
}

func TestFirstLoadOpensOnActiveWork(t *testing.T) {
	var m Model
	m.setNotes([]tasks.Task{{ID: "a", Status: "done"}, {ID: "b", Status: "todo"}, {ID: "c", Status: "build"}})
	if m.notes[m.taskCursor].ID != "c" {
		t.Fatalf("opened on %s, want the in-progress ticket c", m.notes[m.taskCursor].ID)
	}
	m.taskCursor = 0
	m.setNotes([]tasks.Task{{ID: "a", Status: "done"}, {ID: "b", Status: "todo"}})
	if m.notes[m.taskCursor].ID != "a" {
		t.Fatal("a reload must keep the user's selection")
	}
}
