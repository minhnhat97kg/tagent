package board

import (
	"testing"

	"tagent/internal/tasks"
)

func TestClosedTicketNeverFiresOrStarts(t *testing.T) {
	dir, scope := setup(t, gatedConfig+"triggers:\n  - {on: status, agent: pi, prompt: go}\n  - {on: comment, agent: pi, prompt: go}\n")
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := tasks.Add(scope, "Closed", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Move(scope, ticket.ID, "closed", ""); err != nil {
		t.Fatal(err)
	}
	ticket, _ = tasks.Get(scope, ticket.ID)
	for _, ev := range []tasks.Event{
		{Kind: "status", From: "todo", To: "closed", Actor: "user"},
		{Kind: "comment", Actor: "user"},
		{Kind: "mention", Actor: "user", Names: []string{"pi"}},
	} {
		actions, err := Actions(dir, cfg, ev, ticket)
		if err != nil || len(actions) != 0 {
			t.Fatalf("closed ticket fired on %s: %+v %v", ev.Kind, actions, err)
		}
	}
	if err := Start(dir, ticket.ID, ""); err == nil {
		t.Fatal("closed ticket started")
	}
}
