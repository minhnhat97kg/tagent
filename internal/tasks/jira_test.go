package tasks

import (
	"os"
	"testing"
)

func TestOldTaskJSON(t *testing.T) {
	_, scope := testBoard(t)
	task, err := Add(scope, "old", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"id":"` + task.ID + `","text":"Old ticket","status":"todo","parent":"gone","workflow":"old","jobs":{"a":"b"},"outputs":{"x":"y"},"model":"old","timeout_minutes":5,"started_at":"2020-01-01T00:00:00Z"}`
	if err := os.WriteFile(file(scope, task.ID), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := Get(scope, task.ID)
	if err != nil || got.Title != "Old ticket" || len(List(scope)) != 1 {
		t.Fatal(got, err)
	}
}

func TestFinishEntryStaleAndSameStatus(t *testing.T) {
	dir, scope := testBoard(t)
	if err := SaveBoardConfig(dir, []byte("statuses: [{id: todo}, {id: doing}, {id: review}, {id: blocked}, {id: done}]")); err != nil {
		t.Fatal(err)
	}
	task, _ := Add(scope, "ticket", nil)
	Move(scope, task.ID, "doing", "")
	events, _ := ReadEvents(scope, 0)
	seq := events[len(events)-1].Seq
	Move(scope, task.ID, "review", "")
	Move(scope, task.ID, "doing", "")
	if err := FinishEntry(scope, task.ID, "done", "old result", seq, false, MutationOptions{Actor: "ci"}); err != nil {
		t.Fatal(err)
	}
	got, _ := Get(scope, task.ID)
	if got.Status != "doing" || len(got.Comments) != 1 {
		t.Fatal(got)
	}
	before, _ := ReadEvents(scope, 0)
	Move(scope, task.ID, "doing", "")
	after, _ := ReadEvents(scope, 0)
	if len(before) != len(after) {
		t.Fatal("same-status entry")
	}
}

func TestMove(t *testing.T) {
	dir, scope := testBoard(t)
	if err := SaveBoardConfig(dir, []byte("statuses: [{id: todo, next: [build]}, {id: build, next: []}, {id: done}]")); err != nil {
		t.Fatal(err)
	}
	task, _ := Add(scope, "ticket", nil)
	if err := Move(scope, task.ID, "done", "no"); err == nil {
		t.Fatal("invalid transition")
	}
	if err := Move(scope, task.ID, "build", "handoff", MutationOptions{Actor: "ci"}); err != nil {
		t.Fatal(err)
	}
	got, _ := Get(scope, task.ID)
	if len(got.Comments) != 1 || got.Status != "build" {
		t.Fatal(got)
	}
	events, _ := ReadEvents(scope, 0)
	if events[len(events)-1].Actor != "ci" || events[len(events)-2].Kind != "comment" {
		t.Fatal(events)
	}
	if err := Move(scope, task.ID, "done", ""); err == nil {
		t.Fatal("empty next")
	}
	if _, err := Update(scope, task.ID, "done", nil); err != nil {
		t.Fatal(err)
	}
}

func TestStatusConfig(t *testing.T) {
	c, err := ParseBoardConfig(nil)
	if err != nil || len(c.Columns) == 0 || len(c.Statuses) != 0 {
		t.Fatalf("defaults: %+v %v", c, err)
	}
	for _, raw := range []string{"columns: [todo, done]", "statuses: [{id: todo}, {id: done}]"} {
		c, err := ParseBoardConfig([]byte(raw))
		if err != nil || len(c.Statuses) == 0 {
			t.Fatalf("%q: %v", raw, err)
		}
	}
	for _, raw := range []string{"columns: [todo]", "columns: [todo, done]\nstatuses: []", "statuses: [{id: todo, role: pi}, {id: done}]", "statuses: [{id: todo, run: echo}, {id: done}]", "statuses: [{id: todo, next: [missing]}, {id: done}]", "statuses: [{id: todo, max_loops: -1}, {id: done}]"} {
		if _, err := ParseBoardConfig([]byte(raw)); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}
