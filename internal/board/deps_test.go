package board

import (
	"testing"

	"tagent/internal/tasks"
)

const gatedConfig = `statuses:
  - {id: todo}
  - {id: doing, role: implementer}
  - {id: blocked}
  - {id: done}
`

func TestActionsWaitForDependencies(t *testing.T) {
	dir, scope := setup(t, gatedConfig)
	cfg, err := tasks.LoadBoardConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := tasks.Add(scope, "First", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tasks.Add(scope, "Second", nil, tasks.MutationOptions{Needs: []string{first.ID}})
	if err != nil {
		t.Fatal(err)
	}
	entry := tasks.Event{Kind: "status", From: "todo", To: "doing", Actor: "user"}
	actions, err := Actions(dir, cfg, entry, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 0 {
		t.Fatalf("dispatched a waiting ticket: %+v", actions)
	}
	if _, err = tasks.Update(scope, first.ID, "done", nil); err != nil {
		t.Fatal(err)
	}
	actions, err = Actions(dir, cfg, entry, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Role != "implementer" {
		t.Fatalf("ready ticket not dispatched: %+v", actions)
	}
}
