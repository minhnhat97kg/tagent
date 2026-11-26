package config

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"tagent/internal/tasks"
)

// A draft that parses is saved to the project's board.yaml and removed.
func TestFinishBoardSavesValidDraft(t *testing.T) {
	dir := t.TempDir()
	draft := filepath.Join(t.TempDir(), "board.yaml")
	if err := os.WriteFile(draft, []byte("roles: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := uiModel{ed: NewEditor(), board: boardDraft{dir: dir, file: draft}}
	m = m.finishBoard(boardEditedMsg{})
	if _, bad := m.ed.Flash(); bad {
		t.Fatalf("valid draft flashed an error")
	}
	if _, err := os.Stat(tasks.BoardConfigPath(dir)); err != nil {
		t.Fatalf("board.yaml not saved: %v", err)
	}
	if _, err := os.Stat(draft); err == nil {
		t.Fatalf("draft kept after save")
	}
}

// An invalid draft never reaches board.yaml and stays on disk for `e`.
func TestFinishBoardKeepsInvalidDraft(t *testing.T) {
	dir := t.TempDir()
	draft := filepath.Join(t.TempDir(), "board.yaml")
	if err := os.WriteFile(draft, []byte("roles: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := uiModel{ed: NewEditor(), board: boardDraft{dir: dir, file: draft}}
	m = m.finishBoard(boardEditedMsg{})
	if _, bad := m.ed.Flash(); !bad {
		t.Fatalf("invalid draft did not flash an error")
	}
	if _, err := os.Stat(tasks.BoardConfigPath(dir)); err == nil {
		t.Fatalf("invalid draft was written to board.yaml")
	}
	if _, err := os.Stat(draft); err != nil {
		t.Fatalf("rejected draft was deleted: %v", err)
	}
}

// Projects whose paths differ only by / versus _ get different drafts.
func TestDraftPathPerProject(t *testing.T) {
	if draftPath("/x/a_b") == draftPath("/x/a/b") {
		t.Fatalf("draft shared by /x/a_b and /x/a/b")
	}
}

// tab switches between General and Board; the Board tab takes no list keys.
func TestSettingsTabs(t *testing.T) {
	m := uiModel{ed: NewEditor(), width: 80, height: 40}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := next.(uiModel).tab; got != tabBoard {
		t.Fatalf("tab after tab key = %d, want %d", got, tabBoard)
	}
	m2 := next.(uiModel)
	before := m2.ed.Cursor()
	next, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if next.(uiModel).ed.Cursor() != before {
		t.Fatalf("j moved the general list while on the Board tab")
	}
}

// editBoard drafts the saved file for $EDITOR and reopens a kept draft as it is.
func TestEditBoardDraft(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("EDITOR", "true")
	if err := tasks.SaveBoardConfig(dir, []byte("roles: {}\n")); err != nil {
		t.Fatal(err)
	}
	m, cmd := uiModel{ed: NewEditor()}.editBoard()
	if b, err := os.ReadFile(m.board.file); cmd == nil || err != nil || string(b) != "roles: {}\n" {
		t.Fatalf("draft: %q %v (cmd %v)", b, err, cmd != nil)
	}
	if err := os.WriteFile(m.board.file, []byte("roles: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ = m.editBoard()
	if b, _ := os.ReadFile(m.board.file); string(b) != "roles: [\n" {
		t.Fatalf("kept draft was overwritten: %q", b)
	}
}
