package sidebar

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

// commentModel is the board with the first ticket open on the Activity tab.
func commentModel() Model {
	m := boardTestModel()
	m.width, m.height = 110, 40
	m.boardDir = "/repo"
	m.boardDrawer, m.detailTab = true, detailTabActivity
	m.boardConfig.Roles = map[string]tasks.Role{"builder": {Agent: "pi"}, "reviewer": {Agent: "claude"}}
	return m
}

func typeText(m Model, text string) Model {
	for _, r := range text {
		if r == ' ' {
			m, _ = boardPressType(m, tea.KeySpace, nil)
			continue
		}
		m, _ = boardPressType(m, tea.KeyRunes, []rune{r})
	}
	return m
}

func boardPressType(m Model, typ tea.KeyType, runes []rune) (Model, tea.Cmd) {
	next, cmd := m.handleKey(tea.KeyMsg{Type: typ, Runes: runes})
	return next.(Model), cmd
}

func TestCommentBoxSuggestsRolesAndCompletesWithTab(t *testing.T) {
	m := commentModel()
	m, _ = boardPress(m, "c")
	if !m.commentBox {
		t.Fatal("c on Activity did not open the inline composer")
	}
	m = typeText(m, "ping @")
	if got := m.commentOptions(); strings.Join(got, ",") != "builder,reviewer" {
		t.Fatalf("suggestions after @: %v", got)
	}
	m = typeText(m, "re")
	view := ansi.Strip(strings.Join(m.boardDetail(100, 40), "\n"))
	if !strings.Contains(view, "@reviewer") || strings.Contains(view, "@builder]") {
		t.Fatalf("composer does not suggest the matching role:\n%s", view)
	}
	m, _ = boardPressType(m, tea.KeyTab, nil)
	if m.taskInput != "ping @reviewer " {
		t.Fatalf("tab completed to %q", m.taskInput)
	}
	m, _ = boardPress(m, "esc")
	if m.commentBox || m.taskInput != "" {
		t.Fatal("esc did not cancel the composer")
	}
}

func TestCommentBoxSendsOnEnter(t *testing.T) {
	m := commentModel()
	m, _ = boardPress(m, "c")
	m = typeText(m, "looks fine")
	next, cmd := boardPressType(m, tea.KeyEnter, nil)
	if next.commentBox || cmd == nil {
		t.Fatalf("enter did not send: open=%v cmd=%v", next.commentBox, cmd != nil)
	}
}

func TestCommentBoxIsOnlyOpenOnActivity(t *testing.T) {
	m := commentModel()
	m.detailTab = detailTabDetails
	if next, _ := boardPress(m, "c"); next.commentBox {
		t.Fatal("c opened the composer outside Activity")
	}
}

func TestReplyingRowClearsWhenTheRoleReplies(t *testing.T) {
	m := commentModel()
	m.rows = []Row{{PaneID: "%4", Base: "builder", BoardTarget: "builder#1", Status: tmux.StatusRunning}}
	ticket := tasks.Task{ID: "1", Comments: []tasks.TaskComment{{Author: "user", Body: "@builder take a look"}}}
	view := ansi.Strip(strings.Join(m.taskDetailActivity(ticket, 80), "\n"))
	if !strings.Contains(view, "builder replying…") {
		t.Fatalf("no replying row while the pane works:\n%s", view)
	}
	// A mention of another role does not count, and a reply clears the row.
	ticket.Comments = append(ticket.Comments, tasks.TaskComment{Author: "builder", Body: "done"})
	if view := ansi.Strip(strings.Join(m.taskDetailActivity(ticket, 80), "\n")); strings.Contains(view, "replying") {
		t.Fatalf("replying row stayed after the reply:\n%s", view)
	}
}

func TestMentionMatchesWholeRoleNamesOnly(t *testing.T) {
	if mentions("hi @builder2", "builder") || !mentions("hi @builder.", "builder") || !mentions("@builder", "builder") {
		t.Fatal("mention matched a longer name or missed a whole one")
	}
}

func TestCommentBoxStaysVisibleWhenScrolled(t *testing.T) {
	m := commentModel()
	m, _ = boardPress(m, "c")
	m = typeText(m, "hello")
	m.detailScroll = 1 << 20 // far past the end
	view := ansi.Strip(strings.Join(m.boardDetail(100, 20), "\n"))
	if !strings.Contains(view, "hello") || !strings.Contains(view, "enter send") {
		t.Fatalf("composer scrolled out of the drawer:\n%s", view)
	}
}
