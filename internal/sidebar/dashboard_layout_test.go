package sidebar

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workflow"
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
	card := m.boardPanel("Agents", "1 of 3", m.agentRows(40-4, 10), 40, 12, false)
	for i, l := range strings.Split(card, "\n") {
		if lipgloss.Width(l) != 40 {
			t.Fatalf("line %d is %d cells, want 40: %q", i, lipgloss.Width(l), ansi.Strip(l))
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

func TestDashboardShowsLanesAtEveryWidth(t *testing.T) {
	for _, size := range [][2]int{{60, 40}, {100, 30}, {140, 40}} {
		m := dashSample()
		m.taskFocus, m.agentFocus, m.taskCursor = true, 2, 2
		m.width, m.height = size[0], size[1]
		plain := ansi.Strip(m.dashboardView())
		// ticket 3 is "doing": in progress lane with 3 tickets (doing, review, blocked).
		for _, want := range []string{"☷ Tasks  todo 2 · [in progress 3] · done 1", "Ticket number 3", "● doing", "[4]"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("%v: lacks %q:\n%s", size, want, plain)
			}
		}
		if strings.Contains(plain, "doing · 1") {
			t.Fatalf("%v: raw status columns:\n%s", size, plain)
		}
		g := m.dashGeometry()
		if g.wf.h == 0 || !strings.Contains(plain, "[Workflows] · Agents") {
			t.Fatalf("%v: no Workflows card", size)
		}
		next, card := m.tasksCardClick(5, 2, g.tasks.w, g.tasks.h, 4)
		if !card || next.taskCursor != 2 {
			t.Fatalf("%v: ticket hit-test got %d %v", size, next.taskCursor, card)
		}
		_, spans := m.tasksTitle(4)
		next, _ = m.tasksCardClick(spans[0].x0, 0, g.tasks.w, g.tasks.h, 4)
		if next.taskCursor != 0 {
			t.Fatalf("%v: clicking the todo tab selected %d", size, next.taskCursor)
		}
		// the tab spans sit exactly under the title text of the top border
		top := strings.Split(plain, "\n")[g.tasks.y]
		if got := ansi.Cut(top, g.tasks.x+spans[1].x0, g.tasks.x+spans[1].x0+3); got != "[in" {
			t.Fatalf("%v: span 1 starts at %q in %q", size, got, top)
		}
	}
}

// Lanes header text at three widths (active lane bracketed).
func TestLanesHeaderText(t *testing.T) {
	for _, w := range []int{60, 100, 140} {
		m := dashSample()
		m.width, m.height = w, 40
		title, _ := m.tasksTitle(4)
		if got := ansi.Strip(title); got != "[4] ☷ Tasks  todo 2 · [in progress 3] · done 1" {
			t.Fatalf("width %d: %q", w, got)
		}
	}
}

// Every card is always drawn with a full rounded border, the title inside the
// top edge, equal widths; the set of cards never changes with focus.
func TestDashboardCardSetIsStable(t *testing.T) {
	titles := []string{"[1] Agents", "[2] ▥ Usage", "[3] Agent definitions", "[4] ☷ Tasks", "[5] [Workflows]"}
	for _, size := range [][2]int{{140, 40}, {100, 30}, {60, 60}, {40, 60}, {60, 20}} {
		for _, f := range focusOrder {
			m := dashSample()
			m.agentFocus, m.taskFocus, m.workflowFocus = f, f == 2, f == 4
			m.width, m.height = size[0], size[1]
			plain := ansi.Strip(m.dashboardView())
			for _, title := range titles {
				if !strings.Contains(plain, "─ "+title) {
					t.Fatalf("%v focus %d: card %q missing:\n%s", size, f, title, plain)
				}
			}
			if m.dashGeometry().wide {
				continue
			}
			tops, bottoms := 0, 0
			for i, l := range strings.Split(plain, "\n")[1 : size[1]-1] {
				r := []rune(l)
				if len(r) == 0 {
					continue
				}
				first, last := r[0], r[len(r)-1]
				ok := (first == '╭' && last == '╮') || (first == '│' && last == '│') || (first == '╰' && last == '╯')
				if lipgloss.Width(l) != size[0]-1 || !ok {
					t.Fatalf("%v focus %d line %d is not a full card edge: %q", size, f, i, l)
				}
				if first == '╭' {
					tops++
				}
				if first == '╰' {
					bottoms++
				}
			}
			if tops < 5 || tops != bottoms {
				t.Fatalf("%v focus %d: %d tops %d bottoms", size, f, tops, bottoms)
			}
		}
	}
}

// The focused card grows; unfocused ones keep their rest height instead of leaving empty rows.
func TestAccordionFocusedCardGrows(t *testing.T) {
	m := dashSample()
	m.width, m.height = 40, 60
	for _, f := range focusOrder {
		m.agentFocus, m.taskFocus, m.workflowFocus = f, f == 2, f == 4
		g := m.dashGeometry()
		if f == 0 && g.agents.h > 14 || g.agents.h < 3 || g.tasks.h < 6 || g.wf.h < 3 {
			t.Fatalf("focus %d: agents %d tasks %d wf %d", f, g.agents.h, g.tasks.h, g.wf.h)
		}
		if total := g.agents.h + g.usage.h + g.roles.h + g.tasks.h + g.wf.h + g.comp.h; total != 58 {
			t.Fatalf("focus %d: heights sum to %d, want 58", f, total)
		}
	}
	m.agentFocus, m.taskFocus = 2, true
	if g := m.dashGeometry(); g.agents.h > 6 || g.roles.h != 3 || g.wf.h != 3 || g.tasks.h < 20 {
		t.Fatalf("tasks focus: %+v", g)
	}
}

func TestTabCyclesEveryCardBothWays(t *testing.T) {
	m := dashSample()
	m.width, m.height = 100, 30
	want := []int{3, 1, 2, 4, 0}
	for _, f := range want {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
		if m.agentFocus != f || m.taskFocus != (f == 2) || m.workflowFocus != (f == 4) {
			t.Fatalf("Tab: focus %d, want %d", m.agentFocus, f)
		}
	}
	for i := len(want) - 2; i >= -1; i-- {
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
		m = next.(Model)
		if got := want[(i+len(want))%len(want)]; m.agentFocus != got {
			t.Fatalf("Shift+Tab: focus %d, want %d", m.agentFocus, got)
		}
	}
}

// A refresh that re-sorts tickets or workflows keeps each cursor on the same id/name.
func TestCursorsKeepIdentityAfterResort(t *testing.T) {
	m := dashSample()
	m.taskCursor = 2 // ticket "3"
	rev := slices.Clone(m.notes)
	slices.Reverse(rev)
	m.setNotes(rev)
	if m.notes[m.taskCursor].ID != "3" {
		t.Fatalf("ticket cursor moved to %s", m.notes[m.taskCursor].ID)
	}
	m.boardWorkflows = []workflow.Workflow{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	m.workflowCursor = 1
	m.setWorkflows([]workflow.Workflow{{Name: "c"}, {Name: "b"}, {Name: "a"}, {Name: "new"}})
	if m.boardWorkflows[m.workflowCursor].Name != "b" {
		t.Fatalf("workflow cursor on %s", m.boardWorkflows[m.workflowCursor].Name)
	}
	before := m.agentFocus
	m.setNotes(nil)
	if m.agentFocus != before || m.taskCursor != 0 {
		t.Fatal("refresh changed focus or cursor")
	}
}

func TestPositionIndicators(t *testing.T) {
	m := dashSample()
	m.width, m.height, m.taskCursor = 100, 40, 2
	m.taskFocus, m.agentFocus = true, 2
	plain := ansi.Strip(m.dashboardView())
	for _, want := range []string{"1 of 3 ─╯", "1 of 3 ─╯"} { // agents (3 rows), in-progress lane (doing is first of 3)
		if !strings.Contains(plain, want) {
			t.Fatalf("no %q:\n%s", want, plain)
		}
	}
}
