package sidebar

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func dashSample() Model {
	m := boardTestModel()
	m.boardMode, m.taskFocus, m.agentFocus, m.taskCursor = false, false, 0, 2
	m.notesScope = "tmux-agent-12345678"
	m.notes = nil
	for i, st := range []string{"todo", "todo", "doing", "review", "blocked", "done"} {
		m.notes = append(m.notes, tasks.Task{ID: fmt.Sprint(i + 1), Title: fmt.Sprintf("Ticket number %d with a longer title", i+1), Status: st, Assignees: []string{"builder"}})
	}
	m.notes[2].Comments = []tasks.TaskComment{{Author: "user", Body: "please rebase\nthen retry"}}
	m.boardConfig = tasks.BoardConfig{Columns: []string{"todo", "doing", "review", "blocked", "done"}}
	m.rows = []Row{
		{Base: "claude-main", Status: tmux.StatusRunning, GitBranch: m.icons.GitBranch + "main"},
		{Base: "codex-fix", Status: tmux.StatusWaiting},
		{Base: "plain", Status: tmux.StatusIdle},
	}
	return m
}

func TestDashboardCardLinesHaveEqualWidth(t *testing.T) {
	m := dashSample()
	m.width, m.height = 140, 40
	for _, c := range []struct{ title, hint string }{{"Agents · 3", ""}, {"#1 · todo", ""}} {
		card := m.dashboardPanel(c.title, m.agentRows(40-4, 10), 40, 12, false)
		lines := strings.Split(card, "\n")
		for i, l := range lines {
			if lipgloss.Width(l) != 40 {
				t.Fatalf("%s line %d is %d cells, want 40: %q", c.title, i, lipgloss.Width(l), ansi.Strip(l))
			}
		}
	}
}

func TestDashboardRenders(t *testing.T) {
	m := dashSample()
	for _, s := range [][2]int{{140, 40}, {100, 30}, {70, 40}, {60, 20}} {
		m.width, m.height = s[0], s[1]
		out := m.dashboardView()
		if os.Getenv("DASH_PRINT") != "" {
			fmt.Printf("=== %dx%d\n%s\n", s[0], s[1], ansi.Strip(out))
		}
		lines := strings.Split(out, "\n")
		if len(lines) != s[1] {
			t.Fatalf("%v: %d lines", s, len(lines))
		}
		for i, l := range lines {
			if lipgloss.Width(l) > s[0] {
				t.Fatalf("%v line %d too wide", s, i)
			}
		}
	}
}

func TestNewTicketKeyOpensEditor(t *testing.T) {
	m := dashSample()
	m.taskFocus, m.agentFocus = true, 2
	if _, cmd := boardPress(m, "n"); cmd == nil {
		t.Fatal("n did not start the add flow")
	}
}

func TestDashboardWideShowsBoardColumns(t *testing.T) {
	m := dashSample()
	m.taskFocus, m.agentFocus = true, 2
	m.width, m.height = 140, 40
	out := m.dashboardView()
	plain := ansi.Strip(out)
	for _, h := range []string{"todo · 2", "doing · 1", "review · 1"} {
		if !strings.Contains(plain, h) {
			t.Fatalf("missing column header %q:\n%s", h, plain)
		}
	}
	for i, l := range strings.Split(out, "\n")[1:39] {
		if lipgloss.Width(l) != 139 && lipgloss.Width(l) != 140 {
			t.Fatalf("line %d is %d cells", i, lipgloss.Width(l))
		}
	}
	g := m.dashGeometry()
	// click on the first card of the doing column
	_, start, _, _, cw := m.boardColumnSpan(g.tasks.w)
	if idx, ok := m.boardColumnAt(cw*(1-start)+2, 2, g.tasks.w, g.tasks.h-1); !ok || idx != 2 {
		t.Fatalf("hit-test got %d %v", idx, ok)
	}
	m.width = 100
	if p := ansi.Strip(m.dashboardView()); strings.Contains(p, "doing · 1") || !strings.Contains(p, "Ticket number 1") {
		t.Fatalf("narrow dashboard changed:\n%s", p)
	}
}
