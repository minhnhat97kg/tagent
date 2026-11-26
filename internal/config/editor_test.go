package config

import (
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tagent/internal/board"
	"tagent/internal/tmux"
)

func keys(e *Editor, k string) {
	if k == "enter" {
		e.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
		return
	}
	e.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
}

func TestEditorWorkspaces(t *testing.T) {
	defer tmux.SetRunner(tmux.NewFake())()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	e := NewEditor()
	n := len(Settings)
	if e.Len() != n+1 {
		t.Fatalf("no workspaces: %d rows, want %d", e.Len(), n+1)
	}

	keys(e, "a")
	if !e.Editing() {
		t.Fatal("a must open the add input")
	}
	e.input.SetValue("relative")
	keys(e, "enter")
	if msg, bad := e.Flash(); !bad || msg == "" || e.Len() != n+1 {
		t.Fatalf("invalid path: flash %q bad=%v rows=%d", msg, bad, e.Len())
	}

	keys(e, "a")
	e.input.SetValue(dir)
	keys(e, "enter")
	if msg, bad := e.Flash(); bad || e.Len() != n+2 || !slices.Contains(board.Workspaces(), dir) {
		t.Fatalf("add: flash %q bad=%v rows=%d", msg, bad, e.Len())
	}
	if e.Selected().Kind != AddWorkspace {
		t.Fatal("cursor should stay on the add row after adding")
	}

	e.HandleKey(tea.KeyMsg{Type: tea.KeyUp})
	if e.Selected().Kind != Workspace || e.Selected().Key != dir {
		t.Fatalf("selected %+v, want the workspace", e.Selected())
	}
	keys(e, "e")
	keys(e, "r")
	if e.Editing() || e.Len() != n+2 {
		t.Fatal("e and r must do nothing on a workspace row")
	}
	keys(e, "x")
	if slices.Contains(board.Workspaces(), dir) || e.Len() != n+1 {
		t.Fatalf("remove: rows=%d", e.Len())
	}
	if e.cursor >= e.Len() || e.Selected().Kind != AddWorkspace {
		t.Fatalf("cursor after removing the last workspace: %d of %d", e.cursor, e.Len())
	}
}
