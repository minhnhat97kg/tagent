package sidebar

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tagent/internal/metrics"
	"tagent/internal/tasks"
	"tagent/internal/usage"

	"github.com/charmbracelet/x/ansi"
)

func TestTaskDetailShowsTimeTokensAndNote(t *testing.T) {
	m := NewModel("%9")
	now := time.Now()
	m.metricState = metrics.State{Runs: []metrics.Run{
		{TaskID: "t1", SessionKey: "k1", Agent: "claude", Started: now.Add(-12 * time.Minute), Ended: now.Add(-2 * time.Minute), StartTotal: 100, EndTotal: 45100, HasStart: true, HasEnd: true},
		{TaskID: "t1", SessionKey: "k2", Agent: "codex", Started: now.Add(-30 * time.Second), HasStart: true},
		{TaskID: "other", Agent: "x", Started: now},
	}}
	body := ansi.Strip(strings.Join(m.taskDetailLines(tasks.Task{ID: "t1", Text: "fix it", Status: "done", Note: "added retry"}, 80), "\n"))
	for _, want := range []string{"Usage: 45.0k tokens", "claude  45.0k", "codex", "Time       10m", "45.0k", "Note", "added retry"} {
		if !strings.Contains(body, want) {
			t.Fatalf("detail misses %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "other") {
		t.Fatalf("another task's run leaked in:\n%s", body)
	}
	if plain := strings.Join(m.taskDetailLines(tasks.Task{ID: "none", Text: "x", Status: "todo"}, 80), "\n"); strings.Contains(plain, "Tokens") || strings.Contains(plain, "Note") {
		t.Fatalf("no runs and no note: nothing extra expected:\n%s", plain)
	}
}

func TestTaskDetailSummaryFirst(t *testing.T) {
	m := boardTestModel()
	m.width, m.height = 80, 24
	m.notes[0].Text = "First card\n" + strings.Repeat("a long description sentence ", 60)
	m.boardDrawer = true
	m.detailScroll = 1000000 // scrolled to the very bottom, the head stays put
	lines := m.boardDetail(76, 19)
	if len(lines) != 19 {
		t.Fatalf("drawer height %d", len(lines))
	}
	head := ansi.Strip(lines[1]) // lines[0] is the tab strip
	for _, want := range []string{"#1", "todo", "todo"} {
		if !strings.Contains(head, want) {
			t.Fatalf("summary head misses %q: %q", want, head)
		}
	}
	if !strings.Contains(ansi.Strip(lines[2]), "First card") {
		t.Fatalf("title not right under the head: %q", ansi.Strip(lines[2]))
	}
	view := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"Assignee   builder", "Due", "Created", "overdue"} {
		if !strings.Contains(view, want) {
			t.Fatalf("summary misses %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "j/k scroll") {
		t.Fatalf("drawer repeats the footer hints:\n%s", view)
	}
	// Scrolling only moves the body, never the summary.
	m.detailScroll = 0
	if ansi.Strip(m.boardDetail(76, 19)[1]) != head {
		t.Fatal("summary scrolled away")
	}
}

func TestTaskDetailBlockedLine(t *testing.T) {
	m := boardTestModel()
	m.notes[0].ID, m.notes[0].Status = "b1", "blocked"
	m.notes[0].Comments = []tasks.TaskComment{{ID: "q", Body: "Which database?", Created: time.Now()}}
	m.boardHistory = []tasks.Event{{Task: "b1", Kind: "status", From: "build", To: "blocked", CommentID: "q"}}
	body := ansi.Strip(strings.Join(m.taskDetailLines(m.notes[0], 80), "\n"))
	if !strings.Contains(body, "Blocked: Which database?") {
		t.Fatalf("blocked question missing:\n%s", body)
	}
	if want := "Blocked: reply with @role"; !strings.Contains(ansi.Strip(strings.Join(m.taskDetailLines(tasks.Task{ID: "b2", Status: "blocked"}, 80), "\n")), want) {
		t.Fatalf("fallback %q missing", want)
	}
}

func TestTaskDetailDescriptionCollapse(t *testing.T) {
	m := boardTestModel()
	m.width, m.height = 80, 40
	m.notes[0].Text = "First card\nGoal\n" + strings.Repeat("- a description bullet that needs more than one drawer line to fit before it can end here\n", 6)
	m.boardDrawer = true
	collapsed := ansi.Strip(strings.Join(m.boardDetail(76, 35), "\n"))
	if !strings.Contains(collapsed, "more lines · enter/space expand") {
		t.Fatalf("no expand hint:\n%s", collapsed)
	}
	if shown := strings.Count(collapsed, "• a description bullet"); shown < 3 || shown > detailDescLines {
		t.Fatalf("collapsed description shows %d bullets, want about %d:\n%s", shown, detailDescLines/2, collapsed)
	}
	next, _ := boardPress(m, "enter")
	if !next.detailDescOpen {
		t.Fatal("enter did not expand the description")
	}
	opened := ansi.Strip(strings.Join(next.boardDetail(76, 35), "\n"))
	if !strings.Contains(opened, "enter/space collapse") || strings.Contains(opened, "enter/space expand") {
		t.Fatalf("expanded hint wrong:\n%s", opened)
	}
	next, _ = boardPress(next, " ")
	if next.detailDescOpen {
		t.Fatal("space did not collapse the description")
	}
	// A heading stands out from the wrap-heavy body.
	if !strings.Contains(opened, "Goal") {
		t.Fatalf("heading missing:\n%s", opened)
	}
}

func TestTaskDetailSystemEventsHiddenByDefault(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	m := boardTestModel()
	m.notesScope = tasks.Scope(t.TempDir())
	m.width, m.height = 80, 40
	for _, msg := range []string{"planner: reserved; outcome uncertain (status; seq 1180 trigger -1)", "▶ triggered planner: accepted, not completed"} {
		if err := tasks.DispatchAudit(m.notesScope, "1", msg); err != nil {
			t.Fatal(err)
		}
	}
	m.notes[0].Comments = []tasks.TaskComment{{Author: "builder", Body: "real comment", Created: time.Now()}}
	m.boardDrawer = true
	m.detailTab = detailTabActivity
	hidden := ansi.Strip(strings.Join(m.boardDetail(76, 35), "\n"))
	if !strings.Contains(hidden, "2 system events (dispatch) — s to show") {
		t.Fatalf("system events not folded:\n%s", hidden)
	}
	if strings.Contains(hidden, "outcome uncertain") || strings.Contains(hidden, "▶ triggered") {
		t.Fatalf("dispatch noise shown by default:\n%s", hidden)
	}
	if !strings.Contains(hidden, "real comment") {
		t.Fatalf("comment hidden too:\n%s", hidden)
	}
	next, _ := boardPress(m, "s")
	shown := ansi.Strip(strings.Join(next.boardDetail(76, 35), "\n"))
	if !strings.Contains(shown, "s to hide") || !strings.Contains(shown, "outcome uncertain") {
		t.Fatalf("s did not show the system events:\n%s", shown)
	}
	next.detailSysOpen = false
	next = next.laneStep(1)
	if next.detailSysOpen {
		t.Fatal("navigating kept the folded events open")
	}
}

func TestTaskDetailFitsWidth(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	m := boardTestModel()
	m.notesScope = tasks.Scope(t.TempDir())
	if err := tasks.DispatchAudit(m.notesScope, "1", strings.Repeat("dispatch message ", 20)); err != nil {
		t.Fatal(err)
	}
	m.notes[0].Text = "First card\nAcceptance\n" + strings.Repeat("- a long bullet with 界 wide runes and more words than fit anywhere\n", 10)
	m.notes[0].Comments = []tasks.TaskComment{{Author: "user", Body: strings.Repeat("comment body with 界 ", 20), Created: time.Now(), Attachments: []tasks.Attachment{{Name: "very-long-attachment-name-without-any-break.txt"}}}}
	for tab := range detailTabs {
		m.detailTab = tab
		for _, sys := range []bool{false, true} {
			for _, open := range []bool{false, true} {
				m.detailSysOpen, m.detailDescOpen = sys, open
				for _, w := range []int{12, 20, 40, 60, 100, 160} {
					for _, rows := range []int{4, 8, 24, 40} {
						for _, line := range m.boardDetail(w, rows) {
							if ansi.StringWidth(line) > w {
								t.Fatalf("drawer overflow at %dx%d tab %d: %q", w, rows, tab, ansi.Strip(line))
							}
						}
						for _, line := range m.taskDetailLines(m.notes[0], w) {
							if ansi.StringWidth(line) > w {
								t.Fatalf("detail overflow at width %d tab %d: %q", w, tab, ansi.Strip(line))
							}
						}
					}
				}
			}
		}
	}
}

func TestSharedTaskDetailAndScroll(t *testing.T) {
	m := boardTestModel()
	m.width, m.height = 84, 200
	m.notes = m.notes[:1]
	m.notes[0].Text = "First card\n" + strings.Repeat("long text with 界 ", 100)
	m.notes[0].Comments = nil
	for i := 0; i < 5; i++ {
		m.notes[0].Comments = append(m.notes[0].Comments, tasks.TaskComment{Author: "builder", Body: "comment body", Created: time.Now().Add(time.Duration(i) * time.Minute)})
	}
	m.boardDrawer = true
	board := ansi.Strip(strings.Join(m.boardDetail(80, 194), "\n"))
	for _, section := range []string{"Assignee", "Description", "[Details]", "Activity", "Run"} {
		if !strings.Contains(board, section) {
			t.Fatalf("missing %s:\n%s", section, board)
		}
	}
	m.detailTab = detailTabActivity
	activity := ansi.Strip(strings.Join(m.boardDetail(80, 194), "\n"))
	if !strings.Contains(activity, "Activity · 5") || strings.Contains(activity, "Description ─") {
		t.Fatalf("activity tab shows the wrong body:\n%s", activity)
	}
	m.detailTab = detailTabDetails
	m.height = 20
	top := m.boardDetail(80, 15)
	for _, key := range []string{"j", "G", "g", "k", "pgdown", "pgup", "ctrl+d", "ctrl+u", "down", "up"} {
		before := m.detailScroll
		m, _ = boardPress(m, key)
		if key == "j" && m.detailScroll != before+1 {
			t.Fatal("popup did not scroll")
		}
		if key == "G" && m.detailScroll != m.detailMaxScroll() {
			t.Fatal("popup bottom")
		}
		if key == "g" && m.detailScroll != 0 {
			t.Fatal("popup top")
		}
	}
	m.detailScroll = 3
	scrolled := m.boardDetail(80, 15)
	if ansi.Strip(strings.Join(top[1:], "\n")) == ansi.Strip(strings.Join(scrolled[1:], "\n")) {
		t.Fatal("scroll did not change the body")
	}
	if ansi.Strip(top[0]) != ansi.Strip(scrolled[0]) {
		t.Fatal("scroll moved the summary")
	}
	m.detailScroll = 0
	for _, rows := range []int{1, 2, 5, 200} {
		for _, w := range []int{1, 15, 80} {
			lines := m.detailViewport(m.taskDetailBody(m.notes[0], w), w, rows, 100000)
			if len(lines) != rows {
				t.Fatal("viewport height")
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != w {
					t.Fatalf("viewport width: %q", line)
				}
			}
		}
	}
	m.taskForm, m.boardDrawer = "", false
	m.detailScroll, m.detailDescOpen, m.detailSysOpen = 100, true, true
	m, _ = boardPress(m, "enter")
	if m.detailScroll != 0 || m.detailDescOpen || m.detailSysOpen {
		t.Fatal("opening the drawer did not reset to the top")
	}
}

func TestTaskDetailMetadataAndActivity(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	m := boardTestModel()
	m.notesScope = tasks.Scope(t.TempDir())
	if err := tasks.DispatchAudit(m.notesScope, "detail", "failure: send failed; repair then task retry detail"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	task := tasks.Task{ID: "detail", Title: "Title", Text: "Title", Status: "doing", Due: "2000-01-01", CompletedAt: now, Comments: []tasks.TaskComment{
		{Author: "new", Body: "new body", Created: now, Attachments: []tasks.Attachment{{Name: "trace.txt"}}},
		{Author: "old", Body: "old body", Created: now.Add(-time.Hour)},
	}}
	m.metricState = metrics.State{Runs: []metrics.Run{{TaskID: task.ID, Agent: "builder", Started: now.Add(-time.Hour), Ended: now}}}
	details := ansi.Strip(strings.Join(m.taskDetailLines(task, 80), "\n"))
	for _, want := range []string{"unassigned", "overdue", "done ", "No description", "! no agent is running this task"} {
		if !strings.Contains(details, want) {
			t.Fatalf("details tab misses %q: %s", want, details)
		}
	}
	m.detailTab = detailTabActivity
	folded := ansi.Strip(strings.Join(m.taskDetailLines(task, 80), "\n"))
	if !strings.Contains(folded, "1 system event (dispatch) — s to show") || strings.Contains(folded, "failure: send failed") {
		t.Fatalf("dispatch audit not folded:\n%s", folded)
	}
	m.detailSysOpen = true
	body := ansi.Strip(strings.Join(m.taskDetailLines(task, 80), "\n"))
	for _, want := range []string{"failure: send failed", "task retry detail", DefaultIcons().Attach + " trace.txt"} {
		if !strings.Contains(body, want) {
			t.Fatalf("activity tab misses %q: %s", want, body)
		}
	}
	if strings.Index(body, "old body") > strings.Index(body, "new body") {
		t.Fatal("activity order")
	}
	task.Status = "done"
	m.detailTab = detailTabDetails
	if strings.Contains(ansi.Strip(strings.Join(m.taskDetailLines(task, 80), "\n")), "overdue") {
		t.Fatal("completed task overdue")
	}
}

func TestTaskDetailTabs(t *testing.T) {
	m := boardTestModel()
	m.width, m.height = 80, 24
	m.boardDrawer = true
	view := ansi.Strip(strings.Join(m.boardDetail(76, 19), "\n"))
	for _, want := range []string{"[Details]", "Chat", "Activity", "Run", "Description"} {
		if !strings.Contains(view, want) {
			t.Fatalf("default tab misses %q:\n%s", want, view)
		}
	}
	// 1-4 jump straight to a tab.
	for _, c := range []struct {
		key string
		tab int
	}{{"2", detailTabFiles}, {"3", detailTabNeeds}, {"4", detailTabChat}, {"5", detailTabActivity}, {"6", detailTabRun}, {"1", detailTabDetails}} {
		m, _ = boardPress(m, c.key)
		if m.detailTab != c.tab {
			t.Fatalf("%s selected tab %d, want %d", c.key, m.detailTab, c.tab)
		}
	}
	// tab / shift+tab cycle, wrapping at the ends.
	m, _ = boardPress(m, "tab")
	if m.detailTab != detailTabFiles {
		t.Fatal("tab did not cycle forward")
	}
	m, _ = boardPress(m, "shift+tab")
	if m.detailTab != detailTabDetails {
		t.Fatal("shift+tab did not cycle back")
	}
	m, _ = boardPress(m, "shift+tab")
	if m.detailTab != detailTabRun {
		t.Fatal("shift+tab did not wrap to the last tab")
	}
	m.detailScroll = 9
	m, _ = boardPress(m, "1")
	if m.detailScroll != 0 {
		t.Fatal("switching tab did not reset the scroll")
	}
	// The strip marks the active tab, and the body follows it.
	m.detailTab = detailTabRun
	if body := ansi.Strip(strings.Join(m.taskDetailBody(m.notes[0], 76), "\n")); !strings.Contains(body, "No workflow on this ticket") {
		t.Fatalf("run tab is not the run:\n%s", body)
	}
	// Opening the drawer always starts on Details.
	m.boardDrawer, m.detailTab = false, detailTabRun
	m, _ = boardPress(m, "enter")
	if !m.boardDrawer || m.detailTab != detailTabDetails {
		t.Fatal("opening the drawer did not reset the tab")
	}
	if view := ansi.Strip(strings.Join(m.boardDetail(76, 19), "\n")); !strings.Contains(view, "[Details]") {
		t.Fatalf("re-opened drawer does not mark Details:\n%s", view)
	}
}

func TestTaskDetailRunTab(t *testing.T) {
	m := boardTestModel()
	now := time.Now().UTC()
	cfg := tasks.BoardConfig{Statuses: []tasks.Status{
		{ID: "build", Job: "build", Role: "builder", Next: []string{"review"}},
		{ID: "review", Job: "review", Role: "reviewer", Next: []string{"done"}},
		{ID: "done", Job: "done"},
	}}
	buildEnd := now.Add(-10 * time.Minute)
	reviewEnd := now.Add(-2 * time.Minute)
	entries := []tasks.RunEntry{
		{Seq: 1, Step: "build", Loop: 1, Role: "builder", Harness: "claude", Start: now.Add(-30 * time.Minute), End: &buildEnd, Duration: 20 * time.Minute, Result: "pass"},
		{Seq: 2, Step: "review", Loop: 1, Role: "reviewer", Start: now.Add(-10 * time.Minute), End: &reviewEnd, Duration: 8 * time.Minute, Result: "changes"},
		{Seq: 3, Step: "build", Loop: 2, Role: "builder", Start: now.Add(-2 * time.Minute), Duration: 2 * time.Minute, Result: "current"},
	}
	task := tasks.Task{ID: "1", Workflow: "feature", Status: "build"}
	view := ansi.Strip(strings.Join(m.detailRunLines(task, cfg, entries, 80, now), "\n"))
	for _, want := range []string{
		"3 entries", "total", "started",
		"build 2×", "review 1×", "pass", "changes", "running",
		"build #2 after review: changes",
		"builder · claude", "20m",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("run tab misses %q:\n%s", want, view)
		}
	}
	if empty := ansi.Strip(strings.Join(m.detailRunLines(task, cfg, nil, 80, now), "\n")); !strings.Contains(empty, "has not entered a step yet") {
		t.Fatalf("empty run not explained:\n%s", empty)
	}
	if none := ansi.Strip(strings.Join(m.taskDetailRun(tasks.Task{ID: "x"}, 80), "\n")); !strings.Contains(none, "No workflow on this ticket") {
		t.Fatalf("workflow-less ticket not explained:\n%s", none)
	}
	for _, w := range []int{12, 20, 40, 80, 120} {
		for _, line := range m.detailRunLines(task, cfg, entries, w, now) {
			if ansi.StringWidth(line) > w {
				t.Fatalf("run line overflows %d: %q", w, ansi.Strip(line))
			}
		}
	}
}

func TestDetailRunLabel(t *testing.T) {
	cases := []struct {
		e, prev tasks.RunEntry
		hasPrev bool
		want    string
	}{
		{tasks.RunEntry{Step: "build", Loop: 1}, tasks.RunEntry{}, false, "build"},
		{tasks.RunEntry{Step: "build", Loop: 2}, tasks.RunEntry{}, false, "build #2"},
		{tasks.RunEntry{Step: "build", Loop: 2}, tasks.RunEntry{Step: "review", Result: "blocked"}, true, "build #2 after blocked"},
		{tasks.RunEntry{Step: "build", Loop: 2}, tasks.RunEntry{Step: "build", Result: "fail"}, true, "build #2 after retry"},
		{tasks.RunEntry{Step: "build", Loop: 2}, tasks.RunEntry{Step: "review", Result: "changes"}, true, "build #2 after review: changes"},
	}
	for _, c := range cases {
		if got := detailRunLabel(c.e, c.prev, c.hasPrev); got != c.want {
			t.Fatalf("detailRunLabel = %q, want %q", got, c.want)
		}
	}
}

// Wide panes show the ticket panel as a right pane beside the tickets, and
// every row still fits the pane exactly.
func TestTaskDetailRightPane(t *testing.T) {
	m := boardTestModel()
	m.width, m.height = 120, 24
	m.boardDrawer = true
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if !strings.Contains(strings.Join(lines, "\n"), "Task · esc close") {
		t.Fatalf("drawer is not beside the tickets:\n%s", strings.Join(lines, "\n"))
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > m.width {
			t.Fatalf("row %d is %d wide, pane is %d: %q", i, w, m.width, line)
		}
	}
}

// Back (esc, or i toggling the drawer) from a ticket detail opened on the
// dashboard's Tasks card must land on the dashboard with the same ticket
// selected, never on the board.
func TestTaskDetailBackFromDashboardReturnsHome(t *testing.T) {
	open := map[string]func(*testing.T) Model{
		"enter": func(t *testing.T) Model {
			m := dashCardModel(t)
			m, _ = boardPress(m, "enter")
			return m
		},
		"click": func(t *testing.T) Model { // a click on a card is the same origin
			m := dashCardModel(t)
			g := m.dashGeometry()
			next, _ := m.handleMouse(tea.MouseMsg{X: g.tasks.x + 5, Y: g.tasks.y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			return next.(Model)
		},
	}
	for _, back := range []string{"esc", "i"} {
		for name, openDetail := range open {
			t.Run(back+"/"+name, func(t *testing.T) {
				m := openDetail(t)
				if !m.boardMode || !m.boardDrawer {
					t.Fatalf("the Tasks card did not open the detail: boardMode=%v drawer=%v", m.boardMode, m.boardDrawer)
				}
				selected := m.taskCursor
				m, _ = boardPress(m, back)
				if m.boardMode {
					t.Fatalf("%s from the detail rendered the board, want the dashboard:\n%s", back, ansi.Strip(m.View()))
				}
				if m.boardDrawer {
					t.Fatalf("%s left the detail drawer open", back)
				}
				if m.taskCursor != selected {
					t.Fatalf("selection moved to %d, want %d", m.taskCursor, selected)
				}
				view := ansi.Strip(m.View())
				if strings.Contains(view, "Board · ") {
					t.Fatalf("%s rendered the board header:\n%s", back, view)
				}
				if !strings.Contains(view, "Tasks") || !strings.Contains(view, "Ticket number "+fmt.Sprint(selected+1)) {
					t.Fatalf("%s did not render the dashboard with the selected ticket:\n%s", back, view)
				}
			})
		}
	}
}

// dashCardModel is the dashboard with the Tasks card focused on the second
// ticket, already loaded from disk so opening a detail only touches the view.
func dashCardModel(t *testing.T) Model {
	t.Helper()
	m := dashSample()
	m.width, m.height = 120, 35
	m.boardDir, m.repoDir = m.notesScope, m.notesScope
	m.focus, m.taskCursor = cardTasks, 1
	return m
}

func TestTaskDetailShowsUsage(t *testing.T) {
	m := boardTestModel()
	m.metricState = metrics.State{Sessions: map[string]metrics.Session{"pi:a": {Key: "pi:a", Harness: "pi", Agent: "dev-1", HasUsage: true, Usage: usage.Count{Total: 2500}}}}
	m.boardHistory = []tasks.Event{{Task: "u1", Kind: "dispatch", Role: "dev", Harness: "pi", Model: "gpt", PaneName: "dev-1"}}
	out := ansi.Strip(strings.Join(m.taskDetailLines(tasks.Task{ID: "u1", Title: "T", Status: "doing"}, 80), "\n"))
	if !strings.Contains(out, "Usage: 2.5k tokens") || !strings.Contains(out, "dev  pi/gpt  2.5k") {
		t.Fatal(out)
	}
}
