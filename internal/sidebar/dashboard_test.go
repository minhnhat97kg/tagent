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
		for _, want := range []string{"Overview", "Tasks", "Agents"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%dx%d dashboard misses %q", m.width, m.height, want)
			}
		}
	}
	updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab}) // running → agents
	m = updated.(Model)
	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyTab}) // agents → tasks
	m = updated.(Model)
	if !m.taskFocus {
		t.Fatal("tab did not focus tasks")
	}
	updated, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
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
