package board

import (
	"fmt"
	"strings"
	"testing"
	"tmux-agent/internal/tasks"
)

func TestReentryTokenEstimate(t *testing.T) {
	task := tasks.Task{ID: "abc123", Title: "Build feature", Text: strings.Repeat("d", 4000)}
	full := "Read tmux-agent task show abc123 (description and all comments). " + task.Text
	for i := 0; i < 28; i++ {
		c := tasks.TaskComment{ID: fmt.Sprint(i), Author: "builder", Body: strings.Repeat("h", 600)}
		task.Comments = append(task.Comments, c)
		full += "\n" + tasks.CommentBrief(c)
	}
	after := statusPrompt(task, tasks.Status{ID: "build", Role: "builder"}, tasks.Event{}, []string{"test", "blocked"}, 27)
	estimate := func(s string) int { return (len([]rune(s)) + 3) / 4 }
	t.Logf("28-comment fixture (4 chars/token): before=%d after=%d", estimate(full), estimate(after))
	if estimate(after) >= estimate(full)/4 {
		t.Fatal("insufficient reduction")
	}
}
func TestStatusPromptDelta(t *testing.T) {
	task := tasks.Task{ID: "a", Title: "title", Text: "description unique", Comments: []tasks.TaskComment{{ID: "1", Author: "worker", Body: "old"}, {ID: "2", Author: "system", Body: "audit"}, {ID: "3", Author: "worker", Body: "new handoff"}}}
	s := tasks.Status{ID: "build", Role: "builder", Outputs: []string{"summary"}}
	e := tasks.Event{CommentID: "3"}
	first := statusPrompt(task, s, e, []string{"test", "blocked"}, -1)
	if !strings.Contains(first, task.Text) || !strings.Contains(first, "new handoff") || !strings.Contains(first, "--output summary=value") {
		t.Fatal(first)
	}
	again := statusPrompt(task, s, e, []string{"test"}, 2)
	if strings.Contains(again, task.Text) || strings.Contains(again, "audit") || strings.Contains(again, ": old") || !strings.Contains(again, "new handoff") {
		t.Fatal(again)
	}
}
