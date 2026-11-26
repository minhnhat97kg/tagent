package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"tagent/internal/tasks"
)

// boardDraft is the copy of .tasks/board.yaml open in $EDITOR. The saved file
// is only written once the draft parses. A draft that fails is kept in the
// temp dir, so quitting does not lose it and the next `e` reopens it.
type boardDraft struct {
	dir  string // project root the board belongs to
	file string // the draft
}

type boardEditedMsg struct{ err error }

// draftPath is the draft for one project, in the temp dir. The name is a hash
// of the full path, so two projects never share a draft.
func draftPath(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return filepath.Join(os.TempDir(), "tagent-board-"+hex.EncodeToString(sum[:8])+".yaml")
}

// editBoard opens the draft in $EDITOR. A draft kept from a rejected edit is
// reopened; otherwise a fresh copy of the saved file is made.
func (m uiModel) editBoard() (uiModel, tea.Cmd) {
	dir := tasks.MainRoot(workDir())
	file := draftPath(dir)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		b, err := os.ReadFile(tasks.BoardConfigPath(dir))
		if os.IsNotExist(err) {
			b = []byte(tasks.BoardExample) // the first save creates the file
		} else if err != nil {
			m.ed.SetFlash(err.Error(), true)
			return m, nil
		}
		if err := os.WriteFile(file, b, 0o600); err != nil {
			m.ed.SetFlash(err.Error(), true)
			return m, nil
		}
	}
	m.board = boardDraft{dir: dir, file: file}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", file) // $EDITOR may carry arguments
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return boardEditedMsg{err: err} })
}

// finishBoard saves the draft when it is valid and keeps it when it is not.
func (m uiModel) finishBoard(msg boardEditedMsg) uiModel {
	if msg.err != nil {
		m.ed.SetFlash("editor: "+msg.err.Error(), true)
		return m
	}
	b, err := os.ReadFile(m.board.file)
	if err == nil {
		err = tasks.SaveBoardConfig(m.board.dir, b)
	}
	if err != nil {
		m.ed.SetFlash("board.yaml not saved, press e to edit again: "+err.Error(), true)
		return m
	}
	m.ed.SetFlash("saved · "+tasks.BoardConfigPath(m.board.dir), false)
	os.Remove(m.board.file)
	m.board = boardDraft{}
	return m
}

func workDir() string { cwd, _ := os.Getwd(); return cwd }
