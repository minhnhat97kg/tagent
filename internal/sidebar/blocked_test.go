package sidebar

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"tagent/internal/tasks"
	"testing"
)

func TestBlockedRowQuestion(t *testing.T) {
	m := boardTestModel()
	m.notes = []tasks.Task{{ID: "blocked", Title: "Ticket", Status: "blocked", Comments: []tasks.TaskComment{{ID: "question", Body: "Token nằm\nở đâu?"}, {ID: "reply", Body: "unrelated reply"}}}}
	m.boardHistory = []tasks.Event{{Task: "blocked", Kind: "status", From: "build", To: "blocked", CommentID: "question"}}
	row := m.boardCard(0, 80)
	if len(row) != 3 || !strings.Contains(row[2], "Token nằm ở đâu?") || strings.Contains(row[2], "unrelated") {
		t.Fatal(row)
	}
	for _, width := range []int{1, 5, 12} {
		for _, line := range m.boardCard(0, width) {
			if ansi.StringWidth(line) > width {
				t.Fatal(width, line)
			}
		}
	}
	m.boardHistory = nil
	row = m.boardCard(0, 80)
	if !strings.Contains(row[2], "reply with @role") {
		t.Fatal(row)
	}
}
