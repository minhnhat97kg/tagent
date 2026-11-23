package board

import (
	"tagent/internal/tasks"
	"testing"
)

func TestFreshPolicy(t *testing.T) {
	for _, role := range []int{-1, 0, 1, 6} {
		for _, status := range []int{-1, 0, 1, 4} {
			r := tasks.Role{Agent: "pi"}
			s := tasks.Status{ID: "custom", Role: "worker"}
			want := 3
			if role >= 0 {
				r.FreshAfter = &role
				want = role
			}
			if status >= 0 {
				s.FreshAfter = &status
				want = status
			}
			cfg := tasks.BoardConfig{Roles: map[string]tasks.Role{"worker": r}, Statuses: []tasks.Status{s}}
			actions, err := Actions(t.TempDir(), cfg, tasks.Event{Kind: "status", From: "todo", To: "custom"}, tasks.Task{})
			if err != nil || len(actions) != 1 {
				t.Fatalf("%v %v", actions, err)
			}
			if actions[0].FreshAfter != want || actions[0].Agent != "worker" {
				t.Fatalf("%+v want %d", actions[0], want)
			}
		}
	}
}
