package tasks

import (
	"slices"
	"testing"
)

func TestDeclaredLanes(t *testing.T) {
	c, err := ParseBoardConfig([]byte("lanes:\n  todo: [backlog, todo]\n  in progress: [review]\n  done: [done]\nmax_fires_per_hour: 5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Lane("backlog") != "todo" || c.Lane("review") != "in progress" || c.Lane("other") != "in progress" {
		t.Fatalf("lanes %v", c.LaneStatuses)
	}
	if want := []string{"backlog", "todo", "review", "blocked", "done"}; !slices.Equal(c.Columns, want) {
		t.Fatalf("columns %v, want %v", c.Columns, want)
	}
	for _, bad := range []string{"lanes:\n  nope: [a]\n", "lanes:\n  todo: [a]\n  done: [a]\n"} {
		if _, err := ParseBoardConfig([]byte(bad + "max_fires_per_hour: 5\n")); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
