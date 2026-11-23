package sidebar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func agentsTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	os.MkdirAll(filepath.Join(dir, ".tasks"), 0755)
	os.WriteFile(filepath.Join(dir, ".tasks", "board.yaml"), []byte("# keep me\nroles:\n  planner: {agent: pi, model: old}\n  reviewer: {agent: claude}\nstatuses:\n  - {id: todo, next: [plan]}\n  - {id: plan, role: planner, next: [done, blocked]}\n  - {id: blocked}\n  - {id: done}\n"), 0644)
	return dir
}

func TestAgentsPanelOpensAndListsRoles(t *testing.T) {
	dir := agentsTestDir(t)
	m := Model{boardMode: true, boardDir: dir}
	m, _, handled := m.boardKey("t")
	if !handled || !m.agentsOpen || !m.workflowFocus || len(m.agents) != 2 {
		t.Fatalf("panel not open: %+v", m.agents)
	}
	if !strings.Contains(strings.Join(m.agentStripLines(80), "\n"), "planner") {
		t.Fatal("planner not listed")
	}
	m, _, _ = m.boardKey("j")
	m, _, _ = m.boardKey("enter")
	if !m.agentDetail {
		t.Fatal("enter did not show detail")
	}
	m, _, _ = m.boardKey("t")
	if m.agentsOpen {
		t.Fatal("t did not close")
	}
	if _, _, handled := (Model{boardMode: true, boardDir: dir}).boardKey("o"); handled {
		t.Fatal("o must stay the global expand key in board mode")
	}
}

func TestAgentsPanelModelEditSavesBoard(t *testing.T) {
	dir := agentsTestDir(t)
	m := Model{boardMode: true, boardDir: dir}
	m, _, _ = m.boardKey("t")
	m, _, _ = m.boardKey("k")
	m, _, _ = m.boardKey("m")
	if m.taskForm != "agent-model" || m.taskInput != "old" {
		t.Fatalf("no model form: %q %q", m.taskForm, m.taskInput)
	}
	m.taskInput = "new-model"
	next, _ := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	b, _ := os.ReadFile(filepath.Join(dir, ".tasks", "board.yaml"))
	if !strings.Contains(string(b), "# keep me") || !strings.Contains(string(b), "model: new-model") {
		t.Fatalf("board:\n%s", b)
	}
	if !strings.Contains(m.taskNotice, "new panes") {
		t.Fatalf("no notice: %q", m.taskNotice)
	}
}

func TestAgentsPanelDeleteNeedsConfirm(t *testing.T) {
	dir := agentsTestDir(t)
	m := Model{boardMode: true, boardDir: dir}
	m, _, _ = m.boardKey("t")
	m, _, _ = m.boardKey("x")
	if m.agentDelete != "planner" {
		t.Fatal("no confirm")
	}
	m.width, m.height = 120, 40
	if !strings.Contains(m.boardView(), "Delete agent planner? y / n") {
		t.Fatal("no delete prompt on screen")
	}
	m, _, _ = m.boardKey("n")
	if m.agentDelete != "" {
		t.Fatal("n did not cancel")
	}
	m, _, _ = m.boardKey("x")
	m, _, _ = m.boardKey("y")
	if m.agentDelete != "" {
		t.Fatal("y did not clear confirm")
	}
}

func TestAgentsPanelEditCreatesFileForBoardRole(t *testing.T) {
	dir := agentsTestDir(t)
	m := Model{boardMode: true, boardDir: dir, tmuxPane: "%0"}
	m, _, _ = m.boardKey("t")
	m, _, _ = m.boardKey("j")
	m, cmd, _ := m.boardKey("e")
	if cmd == nil {
		t.Fatalf("no editor: %q", m.taskNotice)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "agents", "reviewer.md")); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	m, _, _ = m.boardKey("k")
	m, cmd, _ = m.boardKey("e")
	if cmd != nil || !strings.Contains(m.taskNotice, "pi agents have no instruction file") {
		t.Fatalf("pi edit: %q", m.taskNotice)
	}
}

func TestAgentStripRowsAreSimple(t *testing.T) {
	m := Model{boardMode: true, boardDir: agentsTestDir(t)}
	m, _, _ = m.boardKey("t")
	rows := ansi.Strip(strings.Join(m.agentStripLines(80), "\n"))
	if !strings.Contains(rows, "planner") || !strings.Contains(rows, "pi      old") || strings.Contains(rows, "board") {
		t.Fatalf("rows:\n%s", rows)
	}
	m.agentDetail = true
	dt := ansi.Strip(strings.Join(m.agentStripLines(80), "\n"))
	if strings.Contains(dt, "path") || strings.Contains(dt, "new panes") || !strings.Contains(dt, "Harness") || !strings.Contains(dt, "Used by") {
		t.Fatalf("detail:\n%s", dt)
	}
}
