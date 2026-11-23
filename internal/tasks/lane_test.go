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
	if got := c.Lanes(); !slices.Equal(got, []string{"todo", "in progress", "done"}) {
		t.Fatalf("lanes %v", got)
	}
	if c.Lane("plan") != "todo" || c.Lane("review") != "in progress" || c.Lane("done") != "done" {
		t.Fatal("lane mapping")
	}
}
