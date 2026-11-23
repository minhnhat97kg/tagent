package board

import (
	"tagent/internal/tasks"
	"testing"
)

func TestRoleEffortActions(t *testing.T) {
	for _, target := range []string{"implementer", "pi"} {
		for _, effort := range []string{"", "high"} {
			cfg, err := tasks.ParseBoardConfig([]byte(jiraConfig))
			if err != nil {
				t.Fatal(err)
			}
			r := cfg.Roles["implementer"]
			r.Effort = effort
			cfg.Roles["implementer"] = r
			cfg.Statuses[1].Model = "override"
			cfg.Triggers = []tasks.Trigger{{On: "comment", Agent: target}, {On: "mention", Agent: target}}
			for _, kind := range []string{"status", "comment", "mention"} {
				ev := tasks.Event{Kind: kind, From: "todo", To: "build", Actor: "human", Names: []string{target}}
				a, err := Actions(t.TempDir(), cfg, ev, tasks.Task{ID: "1"})
				if err != nil {
					t.Fatal(err)
				}
				if len(a) != 1 {
					t.Fatal(a)
				}
				want := effort
				if kind != "status" && target == "pi" {
					want = ""
				}
				if a[0].Effort != want {
					t.Fatal(a)
				}
				if kind == "status" && a[0].Model != "override" {
					t.Fatal(a)
				}
			}
		}
	}
}
