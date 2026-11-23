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
		{ID: "one", Text: "First task", Status: "doing"},
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
		t.Fatal("task selection did not update detail")
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
