package sidebar

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tasks"
	"tagent/internal/workflow"
)

func boardTestModel() Model {
	return Model{icons: DefaultIcons(), dashboard: true, boardMode: true, taskFocus: true, agentFocus: 2,
		width: 160, height: 40, theme: DefaultTheme(), notesScope: "kanban",
		boardConfig: tasks.BoardConfig{Columns: []string{"todo", "done"}},
		notes: []tasks.Task{
			{ID: "1", Title: "First card", Text: "Description", Status: "todo", Assignees: []string{"builder"}, Due: "2020-12-01", Comments: []tasks.TaskComment{{Author: "user", Body: "hello @builder", Attachments: []tasks.Attachment{{Name: "trace.txt"}}}}},
			{ID: "2", Title: "Second card", Status: "todo"},
			{ID: "3", Title: "Third card", Status: "done"},
		},
	}
}
func boardPress(m Model, key string) (Model, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if key == "enter" {
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	}
	if key == "esc" {
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	}
	next, cmd := m.handleKey(msg)
	return next.(Model), cmd
}

func TestBoardListTwoLineRows(t *testing.T) {
	m := boardTestModel()
	m.boardConfig = tasks.BoardConfig{Columns: []string{"todo", "build", "review", "blocked", "done"}}
	m.notes = append(m.notes, tasks.Task{ID: "4", Title: "Fourth card", Status: "build"}, tasks.Task{ID: "5", Title: "Fifth card", Status: "blocked"})
	for _, size := range [][2]int{{119, 40}, {45, 40}} {
		m.width, m.height = size[0], size[1]
		view := ansi.Strip(m.View())
		for _, want := range []string{"○ First card", "◐ Fourth card", "✕ Fifth card", "✓ Third card", "todo · builder", "12-01", "Board 5"} {
			if !strings.Contains(view, want) {
				t.Errorf("%v: view lacks %q:\n%s", size, want, view)
			}
		}
		if strings.Contains(view, "Todo 2") || strings.Contains(view, "Owner:") {
			t.Fatalf("%v: still grouped into columns: %s", size, view)
		}
		// Done tickets sort last.
		if strings.Index(view, "Third card") < strings.Index(view, "Fifth card") {
			t.Fatalf("done ticket not last: %s", view)
		}
	}
	for i := range m.notes {
		if rows := m.boardRow(i, 30); len(rows) != 2 {
			t.Fatalf("row %d has %d lines", i, len(rows))
		}
	}
	m.notes[0].Title = strings.Repeat("very long title ", 20)
	for _, line := range m.boardRow(0, 30) {
		if ansi.StringWidth(line) != 30 {
			t.Fatalf("row not fitted: %q", line)
		}
	}
}

func TestBoardPaneBounds(t *testing.T) {
	for _, size := range [][2]int{{95, 20}, {95, 40}, {200, 25}, {60, 15}, {59, 15}, {19, 5}} {
		for _, drawer := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/drawer=%t", size[0], size[1], drawer), func(t *testing.T) {
				m := boardTestModel()
				m.width, m.height, m.boardDrawer = size[0], size[1], drawer
				m.boardConfig.Columns = tasks.DefaultColumns()
				m.notes[0].Title = "A title with 界 and enough words to wrap over many lines"
				m.notes[0].Text = strings.Repeat("long description with @mentions 界 ", 300)
				lines := strings.Split(ansi.Strip(m.View()), "\n")
				if len(lines) != m.height {
					t.Fatalf("got %d lines, want %d", len(lines), m.height)
				}
				maxWidth := 0
				for _, line := range lines {
					maxWidth = max(maxWidth, ansi.StringWidth(line))
					if ansi.StringWidth(line) > m.width {
						t.Fatalf("overflow: %q", line)
					}
				}
				if maxWidth != m.width {
					t.Fatalf("max line width %d, want %d", maxWidth, m.width)
				}
				if !strings.HasPrefix(lines[0], "Board ·") || (!strings.Contains(lines[m.height-2], "enter") && !strings.Contains(lines[m.height-2], "esc")) || strings.Contains(lines[m.height-1], "Triggers:") {
					t.Fatalf("header or footers lost: %v", lines)
				}
				if m.width < 20 {
					return
				}
				// The body, not just padded footer text, must span the entire pane.
				if !strings.HasSuffix(lines[1], "╮") || !strings.HasSuffix(lines[m.height-3], "╯") {
					t.Fatalf("box does not span the full width: %v", lines)
				}
			})
		}
	}
}

func TestBoardDetailDrawerAndFields(t *testing.T) {
	m := boardTestModel()
	m.width = 70 // medium with two configured columns
	for _, key := range []string{"enter", "i"} {
		next, _ := boardPress(m, key)
		if !next.boardDrawer || next.taskForm != "" {
			t.Fatalf("%s did not open drawer", key)
		}
		if view := ansi.Strip(next.View()); !strings.Contains(view, "hello @builder") || !strings.Contains(view, "esc close") {
			t.Fatalf("missing drawer detail: %s", view)
		}
		closed, _ := boardPress(next, "esc")
		if closed.boardDrawer || !closed.boardMode {
			t.Fatal("Esc did not return to Board")
		}
		closed, _ = boardPress(next, key)
		if closed.boardDrawer || !closed.boardMode {
			t.Fatal("detail key did not toggle drawer closed")
		}
	}
	m.notes = []tasks.Task{{Title: "Only title", Text: "Only title\nThe description", Status: "todo"}}
	detail := ansi.Strip(strings.Join(m.boardDetail(40, 30), "\n"))
	if strings.Count(detail, "Only title") != 1 || !strings.Contains(detail, "The description") {
		t.Fatalf("description repeats title: %s", detail)
	}
	if strings.Contains(detail, "due") || strings.Contains(detail, "id ") {
		t.Fatalf("empty detail fields: %s", detail)
	}
}

func TestBoardEntryExitAndTaskForms(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	m := boardTestModel()
	m.width = 70
	m.boardMode, m.repoDir = false, dir
	if !strings.Contains(ansi.Strip(m.View()), "n new") {
		t.Fatal("Tasks footer lacks board")
	}
	m, _ = boardPress(m, "b")
	if !m.boardMode {
		t.Fatal("b did not enter board")
	}
	for _, key := range []string{"s"} {
		next, _ := boardPress(m, key)
		want := key
		if key == "s" {
			want = "start"
		}
		if next.taskForm != want {
			t.Fatalf("%s did not open existing form", key)
		}
		if key == "e" && next.taskInput != m.notes[0].Text {
			t.Fatal("edit did not use selected card")
		}
		next, _ = boardPress(next, "esc")
		if !next.boardMode || next.taskForm != "" {
			t.Fatal("form did not return to Board")
		}
	}
	next, _ := boardPress(m, "x")
	if !next.taskDelete {
		t.Fatal("x did not confirm task deletion")
	}
	for _, key := range []string{"q", "b", "esc"} {
		next, cmd := boardPress(m, key)
		if next.boardMode || cmd != nil {
			t.Fatalf("%s did not return to dashboard", key)
		}
	}
	m.boardMode = false
	m, _ = boardPress(m, "k") // select the Tasks title
	m, _ = boardPress(m, "enter")
	if !m.boardMode {
		t.Fatal("Enter on title did not enter Board")
	}
	m.boardMode, m.notes = false, nil
	m, _ = boardPress(m, "enter")
	if !m.boardMode {
		t.Fatal("Enter on empty Tasks did not enter Board")
	}
}

func TestBoardMoveAndSelection(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	first, err := tasks.Add("kanban", "First", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tasks.Add("kanban", "Second", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Update("kanban", second.ID, "done", nil); err != nil {
		t.Fatal(err)
	}
	m := boardTestModel()
	m.width = 70
	m.notes = tasks.List("kanban")
	m, _ = boardPress(m, "j")
	if m.notes[m.taskCursor].ID != second.ID {
		t.Fatal("j selected wrong task")
	}
	m, cmd := boardPress(m, "H")
	if cmd == nil {
		t.Fatal("H did not mutate status")
	}
	next, _ := m.Update(cmd())
	m = next.(Model)
	updated, err := tasks.Get("kanban", second.ID)
	if err != nil || updated.Status != "todo" {
		t.Fatalf("H move: %+v %v", updated, err)
	}
	if m.notes[m.taskCursor].ID != second.ID {
		t.Fatal("move lost selected task")
	}
	events, err := tasks.ReadEvents("kanban", 0)
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if last.Kind != "status" || last.Actor != "user" || last.To != "todo" {
		t.Fatalf("move event: %+v", last)
	}
	m, _ = boardPress(m, "k")
	if m.notes[m.taskCursor].ID != first.ID {
		t.Fatal("k did not select previous card")
	}
	m.notes = append(m.notes, tasks.Task{Title: "Unknown", Status: "legacy"})
	m, _ = boardPress(m, "j")
	m, _ = boardPress(m, "j")
	if m.taskCursor != 2 || !strings.Contains(ansi.Strip(m.View()), "Unknown") {
		t.Fatal("unknown-status task is not reachable")
	}
}

func TestBoardDetailScrollAndWorker(t *testing.T) {
	m := boardTestModel()
	m.boardDrawer = true
	m.rows = []Row{{PaneID: "%1", Base: "builder", Status: "running"}}
	m.notes[0].Text = "First card\n" + strings.Repeat("description line\n", 80)
	m.notes[0].Comments = []tasks.TaskComment{{Author: "user", Body: strings.Repeat("comment line\n", 40) + "LAST COMMENT LINE"}}
	for _, size := range [][2]int{{95, 22}, {95, 40}, {150, 40}} {
		m.width, m.height = size[0], size[1]
		m.detailScroll = 0
		top := ansi.Strip(strings.Join(m.boardDetail(m.width-4, m.height-5), "\n"))
		if !strings.Contains(top, "Assignee   builder") || !strings.Contains(top, "● builder running") || strings.Contains(top, "LAST COMMENT LINE") {
			t.Fatal(top)
		}
		m, _ = boardPress(m, "j")
		if m.detailScroll != 1 {
			t.Fatal("j did not scroll")
		}
		m, _ = boardPress(m, "G")
		lines := m.boardDetail(m.width-4, m.height-5)
		if !strings.Contains(ansi.Strip(strings.Join(lines, "\n")), "LAST COMMENT LINE") {
			t.Fatal("G missed last comment")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > m.width-4 {
				t.Fatal("overflow")
			}
		}
		m, _ = boardPress(m, "g")
		if m.detailScroll != 0 {
			t.Fatal("g")
		}
	}
	m.rows[0].Status = "idle"
	if m.boardWorker(m.notes[0]) != "○ builder idle" {
		t.Fatal("idle worker")
	}
	m.rows[0].Base = "other-pane" // a reused pane answers for its board target
	m.rows[0].BoardTarget = "builder"
	if m.boardWorker(m.notes[0]) != "○ builder idle" {
		t.Fatal("target worker not matched")
	}
	m.rows = nil
	if m.boardWorker(m.notes[0]) != "" {
		t.Fatal("phantom worker")
	}
	for _, w := range []int{1, 15, 50, 100} {
		for _, r := range []int{1, 5, 30} {
			for _, line := range m.boardDetail(w, r) {
				if ansi.StringWidth(line) > w {
					t.Fatal("overflow")
				}
			}
		}
	}
}

func TestDetailRelativeTime(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		age  time.Duration
		want string
	}{{-time.Minute, "just now"}, {30 * time.Second, "just now"}, {5 * time.Minute, "5m ago"}, {2 * time.Hour, "2h ago"}, {72 * time.Hour, "3d ago"}} {
		if got := detailRelativeTime(now.Add(-tc.age), now); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.age, got, tc.want)
		}
	}
	if got := detailRelativeTime(time.Time{}, now); got != "—" {
		t.Errorf("zero timestamp: %q", got)
	}
}

func TestBoardConfigEditor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "repo with spaces")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("EDITOR", "")
	m := boardTestModel()
	m.boardDir = dir
	_, cmd := boardPress(m, "c")
	if cmd == nil {
		t.Fatal("c did not launch editor")
	}
	path := tasks.BoardConfigPath(dir)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != tasks.BoardExample {
		t.Fatalf("starter config: %q %v", data, err)
	}
	editor, err := boardConfigEditor(dir)
	if err != nil {
		t.Fatal(err)
	}
	if editor.Args[2] != `vi "$1"` {
		t.Fatalf("vi fallback: %v", editor.Args)
	}
	script := filepath.Join(t.TempDir(), "editor script.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' 'columns: [todo, done]' 'triggers:' '  - on: assign' '    when: {to: planner}' '    agent: planner' > \"$1\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", fmt.Sprintf("sh %q", script))
	editor, err = boardConfigEditor(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := editor.Run(); err != nil {
		t.Fatal(err)
	}
	next, _ := m.Update(reloadBoardConfig(dir, nil))
	m = next.(Model)
	if len(m.boardConfig.Columns) != 2 || m.taskNotice != "✓ Board config reloaded" {
		t.Fatalf("reload failed: %+v %s", m.boardConfig, m.taskNotice)
	}
	if strings.Contains(ansi.Strip(m.View()), "Triggers:") {
		t.Fatal("trigger summary missing")
	}
	if err := os.WriteFile(path, []byte("columns: [todo]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := boardConfigEditor(dir); err != nil {
		t.Fatal(err)
	}
	next, _ = m.Update(reloadBoardConfig(dir, nil))
	m = next.(Model)
	if !strings.Contains(m.taskNotice, "columns must include todo and done") || len(m.boardConfig.Columns) != 2 {
		t.Fatalf("validation error missing: %s", m.taskNotice)
	}
}

func TestBoardScrollAndRecentComments(t *testing.T) {
	m := boardTestModel()
	m.width = 70
	m.notes, m.height = nil, 20
	for i := 0; i < 12; i++ {
		m.notes = append(m.notes, tasks.Task{Title: fmt.Sprintf("Card %02d", i), Status: "todo"})
	}
	m.notes = append(m.notes, tasks.Task{Title: "Completed", Status: "done"})
	for i := 0; i < 10; i++ {
		m, _ = boardPress(m, "j")
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Card 10") || strings.Contains(view, "Card 00") {
		t.Fatalf("selected card not visible: %s", view)
	}
	m.height, m.taskCursor = 24, 0
	m.notes[0].Text = strings.Repeat("description ", 300)
	m.width = 160
	m.detailScroll = 1000000
	m.boardDrawer = true // Wide columns open details only in the drawer.
	m.notes[0].Comments = []tasks.TaskComment{{Author: "old", Body: strings.Repeat("old body ", 100), Created: time.Unix(1, 0)}, {Author: "new", Body: "newest @builder", Created: time.Unix(2, 0)}}
	view := ansi.Strip(m.View())
	if newest, old := strings.Index(view, "newest @builder"), strings.Index(view, "old body"); newest < 0 || (old >= 0 && old > newest) {
		t.Fatalf("recent comment hidden: %s", view)
	}
}

func TestBoardWorkflowView(t *testing.T) {
	m := boardTestModel()
	cfg, err := tasks.ParseBoardConfig([]byte(`roles:
  planner: {agent: pi, model: gpt}
statuses:
  - {id: todo, next: [plan]}
  - {id: plan, role: planner, next: [test, blocked]}
  - {id: test, run: go test, pass: done, fail: plan}
  - {id: blocked}
  - {id: done}
`))
	if err != nil {
		t.Fatal(err)
	}
	m.boardConfig = cfg
	m, _ = boardPress(m, "w")
	view := ansi.Strip(m.View())
	for _, want := range []string{"Workflow", "○ todo", "(2)", "→ plan", "◐ plan     planner · pi gpt", "→ test, blocked", "ci: go test", "pass → done · fail → plan", "waiting for you", "✓ done", "esc close"} {
		if !strings.Contains(view, want) {
			t.Errorf("workflow view lacks %q:\n%s", want, view)
		}
	}
	m, _ = boardPress(m, "esc")
	if m.boardFlow || !m.boardMode {
		t.Fatal("esc did not return to the board list")
	}
}

func TestBoardWorkerFollowsStatusRole(t *testing.T) {
	m := boardTestModel()
	m.boardConfig = tasks.BoardConfig{Statuses: []tasks.Status{{ID: "todo"}, {ID: "plan", Role: "planner"}, {ID: "build", Role: "implementer"}, {ID: "done"}}}
	tk := tasks.Task{ID: "t1", Status: "build"}
	m.rows = []Row{{PaneID: "%1", Base: "planner-t1", BoardTarget: "planner#t1", Status: "idle"}, {PaneID: "%2", Base: "implementer-t1", BoardTarget: "implementer#t1", Status: "running"}}
	if got := ansi.Strip(m.boardWorker(tk)); got != "● implementer running" {
		t.Fatalf("worker = %q", got)
	}
	m.notes = []tasks.Task{tk}
	m.boardFlow = true
	if view := ansi.Strip(m.View()); !strings.Contains(view, "implementer · ● 1 working") || !strings.Contains(view, "planner · ○ idle") {
		t.Fatalf("workflow view lacks agent state:\n%s", view)
	}
}

func TestBoardListsWorkflowsBelow(t *testing.T) {
	m := boardTestModel()
	m.boardWorkflows = []workflow.Workflow{{Name: "research", Jobs: make([]workflow.Job, 1)}, {Name: "deploy", Global: true, Jobs: make([]workflow.Job, 3)}}
	m.notes = append(m.notes, tasks.Task{ID: "9", Title: "Run", Status: "researching", Workflow: "research"})
	for _, w := range []int{160, 45} {
		m.width = w
		view := ansi.Strip(m.View())
		for _, want := range []string{"Workflows 2", "▸ research", "local · 1 jobs", "● 1 running", "▸ deploy", "global · 3 jobs"} {
			if !strings.Contains(view, want) {
				t.Errorf("width %d lacks %q:\n%s", w, want, view)
			}
		}
		if strings.Index(view, "First card") > strings.Index(view, "Workflows 2") {
			t.Fatal("workflows are not below the ticket list")
		}
	}
}

func TestBoardColumnsAndNavigation(t *testing.T) {
	m := boardTestModel()
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "todo · 2") || !strings.Contains(view, "done · 1") {
		t.Fatal(view)
	}
	lines := strings.Split(view, "\n")
	if !strings.Contains(lines[1], "todo") || !strings.Contains(lines[1], "done") {
		t.Fatal("headers not side by side")
	}
	for _, tc := range []struct {
		key    string
		cursor int
	}{{"j", 1}, {"j", 1}, {"l", 2}, {"k", 2}, {"h", 0}, {"j", 1}, {"k", 0}} {
		m, _ = boardPress(m, tc.key)
		if m.taskCursor != tc.cursor {
			t.Fatalf("%s: cursor %d want %d", tc.key, m.taskCursor, tc.cursor)
		}
	}
	for _, key := range []string{"L", "H"} {
		m.taskCursor = 0
		if key == "H" {
			m.taskCursor = 2
		}
		_, cmd := boardPress(m, key)
		if cmd == nil {
			t.Fatalf("%s no status mutation", key)
		}
	}
	if len(m.boardCard(2, 40)) != 1 || len(m.boardCard(0, 40)) != 3 {
		t.Fatal("card heights")
	}
	m.width = 119
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "todo") || !strings.Contains(view, "○ First card") {
		t.Fatal("missing narrow groups")
	}
	m, _ = boardPress(m, "j")
	m, _ = boardPress(m, "j")
	if m.taskCursor != 2 {
		t.Fatal("narrow must traverse statuses")
	}
}
