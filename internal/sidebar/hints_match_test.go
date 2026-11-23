package sidebar

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/agents"
	"tagent/internal/workflow"
)

// hintKeys pulls the key group of every "key label" item out of a rendered hint
// line: "j/k" is one group, because at the edge of a list only one of them moves.
func hintKeys(line string) [][]string {
	line = strings.ReplaceAll(ansi.Strip(line), " · ", "  ")
	var keys [][]string
	for _, seg := range strings.Split(line, "  ") {
		f := strings.Fields(seg)
		if len(f) == 0 {
			continue
		}
		var group []string
		for _, k := range strings.Split(f[0], "/") {
			switch k {
			case "Tab":
				k = "tab"
			case "S-Tab":
				k = "shift+tab"
			case "Enter":
				k = "enter"
			case "Esc":
				k = "esc"
			case "↑↓":
				group = append(group, "up")
				k = "down"
			}
			if k != "" && k != "…" {
				group = append(group, k)
			}
		}
		if len(group) > 0 {
			keys = append(keys, group)
		}
	}
	return keys
}

func pressKey(m Model, key string) (string, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	}
	next, cmd := m.handleKey(msg)
	return fmt.Sprintf("%#v", next), cmd
}

func lastLines(s string, from int) string {
	l := strings.Split(s, "\n")
	return l[len(l)-from]
}

// Every key a footer hint names must do something in that context.
func TestHintedKeysAreHandled(t *testing.T) {
	flows := []workflow.Workflow{{Name: "research", File: "research.yml", Jobs: make([]workflow.Job, 1)}}
	ags := []agents.Agent{{Name: "planner", Harness: "pi", Source: "board"}}
	base := func() Model {
		m := boardTestModel()
		m.width, m.height, m.boardDir = 200, 40, t.TempDir()
		m.boardWorkflows = flows
		m.agents = ags
		return m
	}
	dash := func(focus int, mod func(*Model)) func() Model {
		return func() Model {
			m := base()
			m.boardMode, m.taskFocus, m.agentFocus, m.chartDays = false, focus == 2, focus, 7
			m.workflowFocus = focus == 4
			if mod != nil {
				mod(&m)
			}
			return m
		}
	}
	board := func(mod func(*Model)) func() Model {
		return func() Model { m := base(); mod(&m); return m }
	}
	dashHint := func(m Model) string { return lastLines(m.dashboardView(), 1) }
	boardHint := func(m Model) string { return lastLines(m.boardView(), 2) }
	// Documented no-ops: keys that need a live pane row, or content to scroll or browse
	// (the fixture has no workflow files and a one-screen ticket).
	needsPane := []string{"enter", "i", "l", "up", "down"}
	cases := []struct {
		name  string
		model func() Model
		hint  func(Model) string
		noop  []string
	}{
		{"dashboard agents", dash(0, nil), dashHint, needsPane},
		{"dashboard usage", dash(3, nil), dashHint, nil},
		{"dashboard tasks", dash(2, nil), dashHint, nil},
		{"dashboard workflows", dash(4, nil), dashHint, nil},
		{"dashboard agents tab", dash(4, func(m *Model) { m.agentsOpen = true }), dashHint, nil},
		{"board tickets", board(func(m *Model) {}), boardHint, nil},
		{"board workflows", board(func(m *Model) { m.workflowFocus = true }), boardHint, nil},
		{"board agents", board(func(m *Model) { m.workflowFocus, m.agentsOpen = true, true }), boardHint, nil},
		{"workflow view", board(func(m *Model) { m.boardFlow, m.flowWorkflow = true, "x.yml" }), boardHint, []string{"["}},
		{"board config", board(func(m *Model) { m.boardFlow, m.boardConfigView = true, true }), boardHint, []string{"j"}},
		{"drawer details", board(func(m *Model) { m.boardDrawer = true }), boardHint, []string{"j"}},
		{"drawer activity", board(func(m *Model) { m.boardDrawer, m.detailTab = true, detailTabActivity }), boardHint, []string{"j"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := c.model()
			keys := hintKeys(c.hint(m))
			if len(keys) < 2 {
				t.Fatalf("no hint keys found in %q", c.hint(m))
			}
			before := fmt.Sprintf("%#v", m)
			for _, group := range keys {
				if group[0] == "?" {
					continue // help is exercised by help_test.go
				}
				handled := slices.ContainsFunc(group, func(k string) bool { return slices.Contains(c.noop, k) })
				for _, k := range group {
					after, cmd := pressKey(m, k)
					handled = handled || after != before || cmd != nil
				}
				if !handled {
					t.Errorf("hinted key %q does nothing (hint %q)", group, ansi.Strip(c.hint(m)))
				}
			}
		})
	}
}
