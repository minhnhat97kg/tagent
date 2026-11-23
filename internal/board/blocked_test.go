package board

import (
	"strings"
	"tagent/internal/tasks"
	"testing"
)

func TestBlockedReplyResetsLoopsAndContext(t *testing.T) {
	dir, scope := setup(t, jiraConfig)
	cfg, _ := tasks.LoadBoardConfig(dir)
	ticket, _ := tasks.Add(scope, "ticket", nil)
	for i := 0; i < 3; i++ {
		tasks.Move(scope, ticket.ID, "build", "")
		tasks.Move(scope, ticket.ID, "review", "")
	}
	tasks.Move(scope, ticket.ID, "blocked", "Old question?")
	if err := tasks.Comment(scope, ticket.ID, "user", "@implementer new answer", nil); err != nil {
		t.Fatal(err)
	}
	ticket, _ = tasks.Get(scope, ticket.ID)
	events, _ := tasks.ReadEvents(scope, 0)
	actions, err := Actions(dir, cfg, events[len(events)-1], ticket)
	if err != nil || len(actions) != 1 || actions[0].BlockReason != "" {
		t.Fatal(actions, err)
	}
	for _, prompt := range []string{actions[0].Prompt, actions[0].ResumePrompt(0), actions[0].ResumePrompt(999)} {
		if !strings.Contains(prompt, "new answer") || strings.Contains(prompt, "Old question?") || !strings.Contains(prompt, "exactly one concrete question") {
			t.Fatal(prompt)
		}
	}
	for i := 0; i < 2; i++ {
		tasks.Move(scope, ticket.ID, "review", "")
		tasks.Move(scope, ticket.ID, "build", "")
	}
	events, _ = tasks.ReadEvents(scope, 0)
	actions, err = Actions(dir, cfg, events[len(events)-1], ticket)
	if err != nil || len(actions) != 1 || actions[0].BlockReason == "" {
		t.Fatal(actions, err)
	}
}
