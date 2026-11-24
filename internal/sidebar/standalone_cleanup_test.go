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

func TestSweepDecisionSparesKeptTerminals(t *testing.T) {
	for _, c := range []struct {
		name, cmd, kept, self string
		prev                  []string
		candidate, kill       bool
	}{
		{"idle shell, first sweep", "zsh", "", "%9", nil, true, false},
		{"idle shell, seen before", "zsh", "", "%9", []string{"%1"}, true, true},
		{"kept terminal is spared", "zsh", "1", "%9", []string{"%1"}, false, false},
		{"keep flag other than 1 is ignored", "zsh", "0", "%9", []string{"%1"}, true, true},
		{"harness pane is not idle", "claude", "", "%9", []string{"%1"}, false, false},
		{"own pane is never touched", "zsh", "", "%1", []string{"%1"}, false, false},
	} {
		candidate, kill := sweepDecision("%1", c.cmd, c.kept, c.self, nil, c.prev)
		if candidate != c.candidate || kill != c.kill {
			t.Errorf("%s: candidate=%v kill=%v, want %v %v", c.name, candidate, kill, c.candidate, c.kill)
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
