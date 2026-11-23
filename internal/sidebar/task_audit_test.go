package sidebar

import (
	"strings"
	"testing"
	"tmux-agent/internal/tasks"
)

func TestTaskAuditTimeline(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	scope := tasks.Scope(t.TempDir())
	ticket, e := tasks.Add(scope, "audit ticket", nil)
	if e != nil {
		t.Fatal(e)
	}
	if e = tasks.DispatchAudit(scope, ticket.ID, "triggered builder"); e != nil {
		t.Fatal(e)
	}
	m := NewModel("%9")
	m.notesScope = scope
	body := strings.Join(m.taskDetailLines(ticket, 100), "\n")
	if !strings.Contains(body, "triggered builder") {
		t.Fatal(body)
	}
	got, _ := tasks.Get(scope, ticket.ID)
	if len(got.Comments) != 0 || strings.Contains(tasks.Brief(got), "triggered builder") {
		t.Fatal(got)
	}
}
