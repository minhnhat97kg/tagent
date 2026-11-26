package sidebar

import (
	"maps"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// maxPress sends one key through handleKey, as the TUI does.
func maxPress(m Model, key string) Model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	}
	next, _ := m.handleKey(msg)
	return next.(Model)
}

func TestMaximizeFillsBodyForEachCard(t *testing.T) {
	for _, s := range [][2]int{{32, 22}, {130, 45}} {
		for _, f := range focusOrder {
			m := dashSample()
			m.width, m.height, m.focus, m.maximized = s[0], s[1], f, true
			lines := strings.Split(m.dashboardView(), "\n")
			if len(lines) != s[1] {
				t.Fatalf("%v card %d: %d lines, want %d", s, f, len(lines), s[1])
			}
			// The focused card's heavy border is the top row of the body.
			if top := ansi.Strip(lines[1]); !strings.HasPrefix(top, "┏") || !strings.HasSuffix(strings.TrimRight(top, " "), "┓") {
				t.Fatalf("%v card %d: body row 1 is not the card's border: %q", s, f, top)
			}
		}
	}
}

func TestMaximizeRestoresTheSameFrame(t *testing.T) {
	m := dashSample()
	m.width, m.height, m.focus = 130, 45, cardTasks
	before := m.dashboardView()
	for _, key := range []string{"esc", "q"} {
		mx := maxPress(m, "Z")
		if !mx.maximized || mx.dashboardView() == before {
			t.Fatalf("Z did not maximize the Tasks card")
		}
		if got := maxPress(mx, key); got.maximized || got.dashboardView() != before {
			t.Fatalf("%s did not restore the previous frame", key)
		}
	}
	if got := maxPress(maxPress(m, "Z"), "Z"); got.maximized {
		t.Fatalf("Z again did not restore")
	}
}

func TestQuitWithoutMaximizeStillQuits(t *testing.T) {
	m := dashSample()
	m.width, m.height = 130, 45
	if _, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Fatalf("q on the plain dashboard should quit")
	}
}

func TestMaximizeBlocksDragAndLayoutEdit(t *testing.T) {
	m := dashSample()
	m.width, m.height = 130, 45
	m.maximized = true
	if m.dragOK() {
		t.Fatalf("drag allowed while maximized")
	}
	if got := maxPress(m, "M"); got.layoutEdit {
		t.Fatalf("M opened the layout editor while maximized")
	}
}

func TestMaximizeKeepsSavedLayoutForRestore(t *testing.T) {
	m := dashSample()
	m.width, m.height = 130, 45
	saved := m.dashGeometry()
	m.maximized, m.focus = true, cardTasks
	if got := m.dashGeometry(); got.tasks.h == 0 || got.agents.h != 0 {
		t.Fatalf("maximized Tasks: agents %v, tasks %v", got.agents, got.tasks)
	}
	m.maximized = false
	if got := m.dashGeometry(); !maps.Equal(got.rects(), saved.rects()) {
		t.Fatalf("restored geometry %+v, want %+v", got.rects(), saved.rects())
	}
}

// The Definitions tab keeps its keys when the Agents card is maximized: esc
// closes the detail first and only then restores the layout.
func TestDefinitionsKeysWhileMaximized(t *testing.T) {
	m := defsModel(t, agentsTestDir(t))
	m, _ = boardPress(m, "Z")
	if !m.maximized || !m.agentsOpen {
		t.Fatal("Z did not maximize the Definitions card")
	}
	m, _ = boardPress(m, "enter")
	if !m.agentDetail {
		t.Fatal("enter did not open the detail while maximized")
	}
	m, _ = boardPress(m, "esc")
	if m.agentDetail || !m.maximized || !m.agentsOpen {
		t.Fatal("esc should close the detail only, keeping the maximized Definitions card")
	}
	m, _ = boardPress(m, "esc")
	if m.maximized || !m.agentsOpen {
		t.Fatal("esc should restore the layout and keep the Definitions tab")
	}
}

// Enter on the Definitions tab opens an agent's detail; it must not jump into the live pane.
func TestStandaloneEnterStaysOffDefinitions(t *testing.T) {
	s := standaloneModel{left: Model{focus: cardAgents, dashboard: true, agentsOpen: true}, paneID: "%1", layout: layoutSplit}
	next, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(standaloneModel).focusRight {
		t.Fatal("Enter jumped into the pane while Definitions is open")
	}
}

// A pending delete confirm keeps esc and q for itself: the layout stays maximized.
func TestDeleteConfirmBlocksRestore(t *testing.T) {
	m := defsModel(t, agentsTestDir(t))
	m, _ = boardPress(m, "Z")
	m, _ = boardPress(m, "x")
	if m.agentDelete == "" {
		t.Fatal("x did not ask to delete the agent")
	}
	for _, key := range []string{"esc", "q"} {
		next, _ := boardPress(m, key)
		if !next.maximized {
			t.Fatalf("%s restored the layout under a pending delete confirm", key)
		}
	}
}
