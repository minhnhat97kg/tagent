package sidebar

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"tagent/internal/metrics"
	"tagent/internal/tasks"
)

func TestDashboardFitsBothWidthsAndKeepsTaskSelection(t *testing.T) {
	m := NewModel("%10")
	m.EnableDashboard()
	m.notesScope = "project-12345678"
	m.notes = []tasks.Task{
		{ID: "one", Text: "First task", Status: "plan"},
		{ID: "two", Text: "Second task", Status: "done", CompletedAt: time.Now()},
	}
	m.metricState = metrics.State{Sessions: map[string]metrics.Session{
		"codex:s1": {ID: "s1", Harness: "codex", Agent: "worker", Status: "closed", HasUsage: true, LastSeen: time.Now()},
	}}
	for _, size := range [][2]int{{32, 22}, {120, 35}} {
		m.width, m.height = size[0], size[1]
		view := m.dashboardView()
		lines := strings.Split(view, "\n")
		if len(lines) != m.height {
			t.Fatalf("%dx%d rendered %d lines", m.width, m.height, len(lines))
		}
		for i, line := range lines {
			if lipgloss.Width(line) > m.width {
				t.Fatalf("line %d exceeds %d cells: %q", i, m.width, line)
			}
		}
		for _, want := range []string{"Usage", "Tasks", "Agents"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%dx%d dashboard misses %q", m.width, m.height, want)
			}
		}
	}
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab}) // agents → usage
	m = updated.(Model)
	if m.agentFocus != 3 {
		t.Fatal("tab did not focus usage")
	}
	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab}) // usage → definitions
	m = updated.(Model)
	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab}) // definitions → tasks
	m = updated.(Model)
	if !m.taskFocus {
		t.Fatal("tab did not focus tasks")
	}
	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}) // wide: next column
	m = updated.(Model)
	if m.taskCursor != 1 || !strings.Contains(m.taskDetail(80)[1], "Second task") {
		t.Fatalf("sel %d focus %d/%v cols %v w %d", m.taskCursor, m.agentFocus, m.taskFocus, m.boardColumns(), m.width)
	}
	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = updated.(Model)
	if !m.taskDelete {
		t.Fatal("task delete was not confirmed")
	}
}

func TestUsageFocusChangesRange(t *testing.T) {
	m := Model{dashboard: true, width: 100, height: 30, agentFocus: 3, chartDays: 7}
	for _, c := range []struct {
		key  string
		want int
	}{{"l", 30}, {"h", 7}, {"h", 1}, {"l", 7}} {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(c.key)})
		m = next.(Model)
		if m.chartDays != c.want {
			t.Fatalf("after %q chartDays=%d, want %d", c.key, m.chartDays, c.want)
		}
	}
	m.agentFocus = 0 // h/l must not change the range outside the Usage card
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if got := next.(Model).chartDays; got != 7 {
		t.Fatalf("l outside Usage changed range to %d", got)
	}
}

// Narrow (stacked) layout: the Tasks card starts at x=0, so a click anywhere used to focus Tasks.
func TestNarrowClickFocusesTheCardClicked(t *testing.T) {
	m := dashSample()
	m.width, m.height = 70, 40
	g := m.dashGeometry()
	click := func(x, y int) Model {
		next, _ := m.handleMouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		return next.(Model)
	}
	if got := click(g.agents.x+5, g.agents.y+g.agents.h/2); got.taskFocus || got.agentFocus != 0 {
		t.Fatalf("click in Agents: focus=%d taskFocus=%v", got.agentFocus, got.taskFocus)
	}
	if got := click(g.usage.x+5, g.usage.y+g.usage.h/2); got.agentFocus != 3 || got.taskFocus {
		t.Fatalf("click in Usage: focus=%d taskFocus=%v", got.agentFocus, got.taskFocus)
	}
	if got := click(g.tasks.x+5, g.tasks.y); got.agentFocus != 2 || !got.taskFocus {
		t.Fatalf("click on the Tasks header: focus=%d taskFocus=%v", got.agentFocus, got.taskFocus)
	}
}
