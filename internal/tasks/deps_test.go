package tasks

import (
	"strings"
	"testing"
)

func TestNeedsGateAndValidation(t *testing.T) {
	dir, scope := testBoard(t)
	if err := SaveBoardConfig(dir, []byte("statuses: [{id: todo}, {id: doing}, {id: blocked}, {id: done}]")); err != nil {
		t.Fatal(err)
	}
	first, err := Add(scope, "First", nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Add(scope, "Second", nil, MutationOptions{Needs: []string{first.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err = SetNeeds(scope, first.ID, []string{"nope"}); err == nil {
		t.Fatal("unknown dependency accepted")
	}
	if err = SetNeeds(scope, first.ID, []string{first.ID}); err == nil {
		t.Fatal("self dependency accepted")
	}
	if err = SetNeeds(scope, first.ID, []string{second.ID, second.ID}); err == nil {
		t.Fatal("duplicate dependency accepted")
	}
	if err = SetNeeds(scope, first.ID, []string{second.ID}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle accepted: %v", err)
	}
	if err = Move(scope, second.ID, "doing", ""); err == nil || !strings.Contains(err.Error(), "waiting for #"+first.ID) {
		t.Fatalf("move left todo: %v", err)
	}
	if _, err = Update(scope, second.ID, "doing", nil); err == nil || !strings.Contains(err.Error(), "waiting for") {
		t.Fatalf("update left todo: %v", err)
	}
	waiting, _ := Get(scope, second.ID)
	if waiting.Status != "todo" || CardState(scope, waiting) != "waits #"+first.ID {
		t.Fatalf("waiting ticket: %+v %q", waiting, CardState(scope, waiting))
	}
	if err = SetNeeds(scope, first.ID, []string{second.ID}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("indirect cycle accepted: %v", err)
	}
	if _, err = Update(scope, first.ID, "done", nil); err != nil {
		t.Fatal(err)
	}
	waiting, _ = Get(scope, second.ID)
	if waiting.Status != "todo" {
		t.Fatal("dependency done auto-started the ticket")
	}
	if state := CardState(scope, waiting); state != "ready" {
		t.Fatalf("state = %q, want ready", state)
	}
	if err = Move(scope, second.ID, "doing", ""); err != nil {
		t.Fatal(err)
	}
	if deps := Dependents(scope, first.ID); len(deps) != 1 || deps[0].ID != second.ID {
		t.Fatalf("dependents: %+v", deps)
	}
	// A ticket out of todo no longer waits, so it has no badge.
	if waiting, _ = Get(scope, second.ID); CardState(scope, waiting) != "" {
		t.Fatal("running ticket still shows a dependency badge")
	}
}

func TestAutoStartFlag(t *testing.T) {
	_, scope := testBoard(t)
	task, err := Add(scope, "Auto", nil, MutationOptions{AutoStart: true})
	if err != nil || !task.AutoStart {
		t.Fatal(task, err)
	}
	if err = SetAutoStart(scope, task.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := Get(scope, task.ID); got.AutoStart {
		t.Fatal("auto_start still on after off")
	}
}

func TestPriority(t *testing.T) {
	_, scope := testBoard(t)
	task, err := Add(scope, "Priority", nil, MutationOptions{Priority: "high"})
	if err != nil || task.Priority != "high" {
		t.Fatal(task, err)
	}
	if err = SetPriority(scope, task.ID, "urgent"); err == nil {
		t.Fatal("unknown priority accepted")
	}
	if err = SetPriority(scope, task.ID, "normal"); err != nil {
		t.Fatal(err)
	}
	if got, _ := Get(scope, task.ID); got.Priority != "" {
		t.Fatalf("normal should be stored as the default: %q", got.Priority)
	}
	for p, want := range map[string]int{"high": 0, "": 1, "normal": 1, "low": 2} {
		if got := PriorityRank(p); got != want {
			t.Fatalf("rank(%q) = %d, want %d", p, got, want)
		}
	}
	if NextPriority("") != "high" || NextPriority("high") != "low" || NextPriority("low") != "normal" {
		t.Fatal("priority cycle")
	}
	if PriorityMark("high") != "↑" || PriorityMark("low") != "↓" || PriorityMark("") != "" {
		t.Fatal("priority marks")
	}
}
