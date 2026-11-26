package sidebar

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

// Each pane picks its tier from its own rect, so these pin the breakpoints on
// both sides and the content each tier draws.

func TestTasksSizeBreakpoints(t *testing.T) {
	for _, c := range []struct {
		w, h int
		want tasksSize
	}{
		{63, 5, sizeCounts}, {63, 6, sizeList}, {64, 5, sizeCounts}, {64, 6, sizeDetail},
		{63, 12, sizeList}, {99, 11, sizeDetail}, {99, 12, sizeDetail}, {100, 11, sizeDetail},
		{100, 12, sizeBoard}, {130, 45, sizeBoard}, {2, 3, sizeCounts},
	} {
		if got := tasksSizeFor(c.w, c.h); got != c.want {
			t.Errorf("tasksSizeFor(%d, %d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

func TestTasksCardTiersDrawExactRect(t *testing.T) {
	for _, empty := range []bool{false, true} {
		for _, s := range [][2]int{{2, 3}, {4, 3}, {20, 5}, {63, 6}, {64, 6}, {64, 5}, {99, 11}, {100, 11}, {100, 12}, {130, 45}} {
			m := boardTestModel()
			if empty {
				m.notes = nil
			}
			lines := strings.Split(m.tasksCard(s[0], s[1], 0, true), "\n")
			if len(lines) != s[1] {
				t.Fatalf("%v empty=%t: %d lines, want %d", s, empty, len(lines), s[1])
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != max(4, s[0]) {
					t.Fatalf("%v empty=%t line %d: width %d", s, empty, i, w)
				}
			}
		}
	}
}

func TestTasksCardContentPerTier(t *testing.T) {
	m := boardTestModel() // todo 2, done 1
	counts := ansi.Strip(m.tasksCard(64, 5, 0, false))
	if !strings.Contains(counts, "○ todo 2") || strings.Contains(counts, " of ") {
		t.Errorf("counts tier:\n%s", counts)
	}
	detail := ansi.Strip(m.tasksCard(64, 6, 0, false))
	if !strings.Contains(detail, "│") || !strings.Contains(detail, "First card") {
		t.Errorf("detail tier:\n%s", detail)
	}
	board := ansi.Strip(m.tasksCard(100, 12, 0, false))
	for _, want := range []string{"[todo 2]", "doing 0", "blocked 0", "done 1"} {
		if !strings.Contains(board, want) {
			t.Errorf("board tier missing %q:\n%s", want, board)
		}
	}
}

func TestTasksBoardShowsClosedWhenSelected(t *testing.T) {
	m := boardTestModel()
	m.notes = append(m.notes, tasks.Task{ID: "9", Title: "Shipped", Status: "closed"})
	m.taskCursor = len(m.notes) - 1
	if got := m.tasksBoardLanes(); got[3] != "closed" {
		t.Fatalf("fourth lane = %v, want closed", got)
	}
	m.taskCursor = 0
	if got := m.tasksBoardLanes(); got[3] != "done" {
		t.Fatalf("fourth lane = %v, want done", got)
	}
}

func TestTasksCardClickFollowsTier(t *testing.T) {
	m := boardTestModel() // lanes: todo has 1 and 2, done has 3
	m.taskCursor = 1
	// board tier (100x12): column 0 starts at x 2 and is 23 wide; column 3 (done) at x 74
	if next, card := m.tasksCardClick(74, 2, 100, 12, 0); !card || next.notes[next.taskCursor].ID != "3" {
		t.Fatalf("board card click on done: card=%t cursor=%d", card, next.taskCursor)
	}
	if next, _ := m.tasksCardClick(74, 1, 100, 12, 0); next.activeLane() != "done" {
		t.Fatalf("board header click should select done, got %s", next.activeLane())
	}
	if next, card := m.tasksCardClick(30, 2, 100, 12, 0); card || next.taskCursor != 1 {
		t.Fatalf("empty in-progress column click selected something")
	}
	// detail tier (64x6): the list is x 2..29; x 40 is the detail column
	if next, card := m.tasksCardClick(40, 2, 64, 6, 0); card || next.taskCursor != 1 {
		t.Fatalf("detail column click changed the selection")
	}
	if _, card := m.tasksCardClick(5, 2, 64, 6, 0); !card {
		t.Fatalf("list column click did not select")
	}
	if _, card := m.tasksCardClick(5, 2, 64, 5, 0); card {
		t.Fatalf("counts tier body click should do nothing")
	}
}

func TestAgentsCountsBelowFiveRows(t *testing.T) {
	m := dashSample() // running 1, waiting 1, idle 1
	got := strings.Join(m.agentRows(40, 2), " ")
	if !strings.Contains(got, "3 agents · 1 running · 1 waiting · 1 other") {
		t.Fatalf("agent counts = %q", got)
	}
	// card rect 4 rows (body 2) is counts; 5 rows (body 3) keeps the list
	if strings.Contains(strings.Join(m.agentRows(40, 3), " "), "agents ·") {
		t.Fatalf("a 5-row Agents card should keep the list")
	}
}

func TestWorkflowsWithoutWorkflowsHasNoFlow(t *testing.T) {
	m := boardTestModel()
	m.boardWorkflows = nil
	got := ansi.Strip(m.workflowCard(120, 6, 0))
	if !strings.Contains(got, "No workflows") {
		t.Fatalf("no workflows should show the empty list alone:\n%s", got)
	}
}

// Shrinking the Tasks pane at a fixed terminal width walks the tiers down.
func TestTasksTierFollowsPaneHeightAtFixedWidth(t *testing.T) {
	m := dashSample()
	m.width = 130
	seen := map[tasksSize]bool{}
	prev := sizeCounts
	for h := 8; h <= 45; h++ {
		m.height = h
		g := m.dashGeometry()
		size := tasksSizeFor(g.tasks.w, g.tasks.h)
		if size < prev {
			t.Fatalf("height %d: tier fell back from %d to %d", h, prev, size)
		}
		prev = size
		seen[size] = true
	}
	if !seen[sizeCounts] || !seen[sizeDetail] {
		t.Fatalf("expected counts and detail tiers across heights, got %v", seen)
	}
}

func TestUsageFitsItsRows(t *testing.T) {
	m := dashSample() // has a "Now" line: now, tokens are the base
	for _, c := range []struct{ width, rows, want int }{
		{60, 1, 2}, // base only, even when short
		{60, 2, 2}, // no sparkline row left
		{60, 3, 3}, // sparkline joins
		{60, 5, 3}, // the grid needs 3 more rows
		{60, 6, 6}, // now, grid, tokens, sparkline
		{10, 6, 3}, // the grid is wider than 10 cells
	} {
		if n := len(m.usageLines(c.width, c.rows)); n != c.want {
			t.Errorf("usageLines(%d, %d) = %d lines, want %d", c.width, c.rows, n, c.want)
		}
	}
}

func TestWorkflowFlowBeside80Columns(t *testing.T) {
	m := boardTestModel()
	m.boardWorkflows = []workflow.Workflow{{Name: "flow", Jobs: []workflow.Job{
		{ID: "plan", Status: "todo", Agent: "planner"},
		{ID: "build", Status: "doing", Agent: "builder", Needs: []string{"plan"}},
	}}}
	wide := ansi.Strip(m.workflowCard(80, 8, 0))
	if !strings.Contains(wide, "build") || !strings.Contains(wide, "needs plan") {
		t.Errorf("80 wide: no job flow:\n%s", wide)
	}
	narrow := ansi.Strip(m.workflowCard(79, 8, 0))
	if strings.Contains(narrow, "needs plan") || !strings.Contains(narrow, "flow") {
		t.Errorf("79 wide: flow should be hidden:\n%s", narrow)
	}
	m.boardWorkflows[0].Jobs = append(m.boardWorkflows[0].Jobs, workflow.Job{ID: "ship"}, workflow.Job{ID: "tail"})
	if got := ansi.Strip(m.workflowCard(80, 5, 0)); !strings.Contains(got, "↓ 2 more") {
		t.Errorf("overflow marker missing:\n%s", got)
	}
}

func TestWorkflowFlowClickDoesNothing(t *testing.T) {
	m := dashSample()
	m.width, m.height, m.focus = 130, 45, cardWorkflows
	m.boardWorkflows = []workflow.Workflow{{Name: "one"}, {Name: "two", Jobs: []workflow.Job{{ID: "a"}}}}
	g := m.dashGeometry()
	listW, _ := m.workflowCols(g.wf.w)
	row := g.wf.y + 2 // the second workflow's row
	flow, _ := m.handleMouse(tea.MouseMsg{X: g.wf.x + 2 + listW + 2, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if flow.(Model).workflowCursor != 0 {
		t.Fatalf("flow-column click moved the workflow cursor")
	}
	list, _ := m.handleMouse(tea.MouseMsg{X: g.wf.x + 3, Y: row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if list.(Model).workflowCursor != 1 {
		t.Fatalf("list-column click did not select the workflow")
	}
}
