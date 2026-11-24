package sidebar

import (
	"os"
	"path/filepath"
	"slices"
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
	if !handled || !m.agentsOpen || m.focus != cardWorkflows || len(m.agents) != 2 {
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
	path := filepath.Join(dir, ".tasks", "agents", "reviewer.md")
	if b, err := os.ReadFile(path); err != nil || strings.HasPrefix(string(b), "---") {
		t.Fatalf("file not created: %v %q", err, b)
	}
	m, _, _ = m.boardKey("k")
	m, cmd, _ = m.boardKey("e")
	if cmd == nil {
		t.Fatalf("pi edit: %q", m.taskNotice)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tasks", "agents", "planner.md")); err != nil {
		t.Fatalf("pi file not created: %v", err)
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

// TestAgentsDetailHintsOfferBack covers ticket 44: the basic-info view opened
// with Enter must advertise the key that closes it, in the footer hint line and
// in the ? help section of the pane that owns it, and that key must restore the
// list with the same row selected.
func TestAgentsDetailHintsOfferBack(t *testing.T) {
	for _, c := range []struct {
		name string
		open func(m Model) Model // the panel or tab, list on screen
	}{
		{"board agents", func(m Model) Model {
			m.boardMode = true
			m, _, _ = m.boardKey("t")
			return m
		}},
		{"dashboard agents tab", func(m Model) Model {
			m.dashboard, m.focus, m.agentsOpen = true, cardWorkflows, true
			return m.loadAgents()
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := Model{boardDir: agentsTestDir(t), theme: DefaultTheme(), icons: DefaultIcons(), width: 140, height: 40}
			m = c.open(m)
			m, _, _ = m.boardKey("j") // second agent, so "same row selected" means something
			selected := m.agentCursor
			m, _, _ = m.boardKey("enter")
			if !m.agentDetail {
				t.Fatal("enter did not open the detail")
			}

			foot := ansi.Strip(m.footerHints())
			if !strings.Contains(foot, "esc") {
				t.Errorf("footer while detail is open has no back key: %q", foot)
			}
			var help []string
			for _, kv := range m.paneSection(m.focused()).keys {
				help = append(help, kv[0]+" "+kv[1])
			}
			if !slices.ContainsFunc(help, func(row string) bool {
				return strings.HasPrefix(row, "esc") && strings.Contains(row, "back") && strings.Contains(row, "list")
			}) {
				t.Errorf("? help while detail is open has no esc back-to-list row: %q", help)
			}

			m, _, _ = m.boardKey("esc")
			if m.agentDetail {
				t.Fatal("esc did not close the detail")
			}
			if m.agentCursor != selected {
				t.Fatalf("esc changed the selection: %d, want %d", m.agentCursor, selected)
			}
			lines := m.agentStripLines(100)
			if strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "Harness") {
				t.Fatal("esc did not restore the list")
			}
			if selected >= len(lines) || !strings.Contains(ansi.Strip(lines[selected]), "❯") {
				t.Errorf("row %d is not the marked one: %q", selected, lines)
			}
		})
	}
}

// TestAgentsDetailUsesFullPane covers ticket 52: the agent detail replaces the
// list, so on a short terminal it must take the whole pane, not stay squeezed
// into the list-sized Workflows card (which cut it to its title row).
func TestAgentsDetailUsesFullPane(t *testing.T) {
	m := Model{boardMode: true, boardDir: agentsTestDir(t), theme: DefaultTheme(), icons: DefaultIcons(), width: 100, height: 12}
	m, _, _ = m.boardKey("t")
	m, _, _ = m.boardKey("enter")
	if view := ansi.Strip(m.boardView()); !strings.Contains(view, "Prompt") {
		t.Fatalf("detail is cut off on a short pane:\n%s", view)
	}
}
