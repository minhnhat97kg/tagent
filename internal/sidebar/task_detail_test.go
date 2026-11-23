package sidebar

import (
	"strings"
	"testing"
	"time"

	"tmux-agent/internal/metrics"
	"tmux-agent/internal/tasks"

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
	for _, want := range []string{"claude  10m  45.0k tokens", "codex", "(running)", "Time       10m", "Note", "added retry"} {
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
	m.boardMode = true
	popup := board
	for _, section := range []string{"Details", "Description", "Activity · 5"} {
		if !strings.Contains(board, section) || !strings.Contains(popup, section) {
			t.Fatalf("missing %s", section)
		}
	}
	expected := m.detailViewport(m.taskDetailLines(m.notes[0], 80), 80, 194, 0)
	if board != ansi.Strip(strings.Join(expected, "\n")) {
		t.Fatal("board differs from shared viewport")
	}
	m.height = 20
	top := m.detailViewport(m.taskDetailLines(m.notes[0], 80), 80, 14, 0)
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
	bottom := m.detailViewport(m.taskDetailLines(m.notes[0], 80), 80, 14, 1)
	if top[1] == bottom[1] {
		t.Fatal("scroll did not change first content line")
	}
	for _, rows := range []int{1, 2, 5, 200} {
		for _, w := range []int{1, 15, 80} {
			lines := m.detailViewport(m.taskDetailLines(m.notes[0], w), w, rows, 100000)
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
	m.taskForm = ""
	m.detailScroll = 100
	m, _ = boardPress(m, "enter")
	if m.detailScroll != 0 {
		t.Fatal("opening popup did not reset scroll")
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
	body := ansi.Strip(strings.Join(m.taskDetailLines(task, 80), "\n"))
	for _, want := range []string{"failure: send failed", "task retry detail", "unassigned", "overdue", "Completed", "No description", "! no agent is running this task", DefaultIcons().Attach + " trace.txt"} {
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
