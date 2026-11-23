package sidebar

import (
	"strings"
	"testing"
	"time"

	"tagent/internal/metrics"
	"tagent/internal/tasks"

	"github.com/charmbracelet/x/ansi"
)

func TestTaskDetailShowsTimeTokensAndNote(t *testing.T) {
	m := NewModel("%9")
	now := time.Now()
	m.metricState = metrics.State{Runs: []metrics.Run{
		{TaskID: "t1", Agent: "claude", Started: now.Add(-12 * time.Minute), Ended: now.Add(-2 * time.Minute), StartTotal: 100, EndTotal: 45100, HasStart: true, HasEnd: true},
		{TaskID: "t1", Agent: "codex", Started: now.Add(-30 * time.Second), HasStart: true},
		{TaskID: "other", Agent: "x", Started: now},
	}}
	body := ansi.Strip(strings.Join(m.taskDetailLines(tasks.Task{ID: "t1", Text: "fix it", Status: "done", Note: "added retry"}, 80), "\n"))
	for _, want := range []string{"claude  10m  45.0k tokens", "codex", "(running)", "Time       10m", "45.0k", "Note", "added retry"} {
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
	head := ansi.Strip(lines[0])
	for _, want := range []string{"#1", "todo", "todo"} {
		if !strings.Contains(head, want) {
			t.Fatalf("summary head misses %q: %q", want, head)
		}
	}
	if !strings.Contains(ansi.Strip(lines[1]), "First card") {
		t.Fatalf("title not right under the head: %q", ansi.Strip(lines[1]))
	}
	view := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"Assignee   builder", "Due", "Created", "overdue"} {
		if !strings.Contains(view, want) {
			t.Fatalf("summary misses %q:\n%s", want, view)
		}
	}
	if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "j/k scroll") {
		t.Fatalf("sticky footer missing: %q", ansi.Strip(lines[len(lines)-1]))
	}
	// Scrolling only moves the body, never the summary.
	m.detailScroll = 0
	if ansi.Strip(m.boardDetail(76, 19)[0]) != head {
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
	for _, sys := range []bool{false, true} {
		for _, open := range []bool{false, true} {
			m.detailSysOpen, m.detailDescOpen = sys, open
			for _, w := range []int{12, 20, 40, 60, 100, 160} {
				for _, rows := range []int{4, 8, 24, 40} {
					for _, line := range m.boardDetail(w, rows) {
						if ansi.StringWidth(line) > w {
							t.Fatalf("drawer overflow at %dx%d: %q", w, rows, ansi.Strip(line))
						}
					}
					for _, line := range m.taskDetailLines(m.notes[0], w) {
						if ansi.StringWidth(line) > w {
							t.Fatalf("detail overflow at width %d: %q", w, ansi.Strip(line))
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
	for _, section := range []string{"Assignee", "Description", "Activity · 5"} {
		if !strings.Contains(board, section) {
			t.Fatalf("missing %s:\n%s", section, board)
		}
	}
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
	if ansi.Strip(strings.Join(top[6:], "\n")) == ansi.Strip(strings.Join(scrolled[6:], "\n")) {
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
	folded := ansi.Strip(strings.Join(m.taskDetailLines(task, 80), "\n"))
	if !strings.Contains(folded, "1 system event (dispatch) — s to show") || strings.Contains(folded, "failure: send failed") {
		t.Fatalf("dispatch audit not folded:\n%s", folded)
	}
	m.detailSysOpen = true
	body := ansi.Strip(strings.Join(m.taskDetailLines(task, 80), "\n"))
	for _, want := range []string{"failure: send failed", "task retry detail", "unassigned", "overdue", "done ", "No description", "! no agent is running this task", DefaultIcons().Attach + " trace.txt"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q: %s", want, body)
		}
	}
	if strings.Index(body, "old body") > strings.Index(body, "new body") {
		t.Fatal("activity order")
	}
	task.Status = "done"
	if strings.Contains(ansi.Strip(strings.Join(m.taskDetailLines(task, 80), "\n")), "overdue") {
		t.Fatal("completed task overdue")
	}
}
