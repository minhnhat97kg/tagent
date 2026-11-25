package tasks

import (
	"slices"
	"testing"
)

func TestLanes(t *testing.T) {
	c, err := ParseBoardConfig([]byte("statuses:\n  - {id: todo}\n  - {id: plan, column: todo}\n  - {id: build}\n  - {id: review}\n  - {id: blocked}\n  - {id: done}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Lanes(); !slices.Equal(got, []string{"todo", "in progress", "blocked", "done", "closed"}) {
		t.Fatalf("lanes %v", got)
	}
	// Lanes are fixed: job statuses land in progress and column: is ignored.
	if c.Lane("plan") != "in progress" || c.Lane("build") != "in progress" || c.Lane("blocked") != "blocked" || c.Lane("done") != "done" || c.Lane("closed") != "closed" {
		t.Fatal("lane mapping")
	}
}
