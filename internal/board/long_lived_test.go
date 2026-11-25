package board

import (
	"strings"
	"tagent/internal/tasks"
	"testing"
)

// A long_lived role validates, and its actions carry the flag and no default fresh-after refresh.
func TestLongLivedRoleActions(t *testing.T) {
	longLived := strings.Replace(jiraConfig, "{agent: pi, model: chosen}", "{agent: pi, model: chosen, long_lived: true}", 1)
	cfg, err := tasks.ParseBoardConfig([]byte(longLived))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Roles["implementer"].LongLived {
		t.Fatal("long_lived not parsed")
	}
	status := tasks.Event{Kind: "status", From: "todo", To: "build", Actor: "human", Task: "1"}
	a, err := Actions(t.TempDir(), cfg, status, tasks.Task{ID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || !a[0].LongLived || a[0].FreshAfter != 0 {
		t.Fatal(a)
	}

	// An explicit fresh_after still refreshes the standing pane.
	explicit := strings.Replace(longLived, "long_lived: true", "long_lived: true, fresh_after: 2", 1)
	cfg, err = tasks.ParseBoardConfig([]byte(explicit))
	if err != nil {
		t.Fatal(err)
	}
	a, err = Actions(t.TempDir(), cfg, status, tasks.Task{ID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || !a[0].LongLived || a[0].FreshAfter != 2 {
		t.Fatal(a)
	}

	// A mention of the role dispatches to the same standing role.
	cfg, err = tasks.ParseBoardConfig([]byte(longLived))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Triggers = []tasks.Trigger{{On: "mention", Agent: "implementer"}}
	mention := tasks.Event{Kind: "mention", Actor: "human", Names: []string{"implementer"}, Task: "1"}
	a, err = Actions(t.TempDir(), cfg, mention, tasks.Task{ID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || !a[0].LongLived {
		t.Fatal(a)
	}
}
