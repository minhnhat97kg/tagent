package sidebar

import (
	"slices"
	"strings"
	"testing"
	"time"

	"tagent/internal/tasks"
)

func TestLaneCardsOrderByPriority(t *testing.T) {
	now := time.Now()
	m := boardTestModel()
	m.boardConfig = tasks.BoardConfig{Columns: tasks.DefaultColumns()}
	m.notes = []tasks.Task{
		{ID: "1", Status: "todo", Updated: now},
		{ID: "2", Status: "todo", Priority: "high", Updated: now.Add(-time.Hour)},
		{ID: "3", Status: "todo", Priority: "low", Updated: now.Add(time.Hour)},
		{ID: "4", Status: "todo", Updated: now.Add(-2 * time.Hour)},
	}
	if got := m.laneCards("todo"); !slices.Equal(got, []int{1, 0, 3, 2}) {
		t.Fatalf("lane order = %v, want high, recent normal, older normal, low", got)
	}
	m.notes[0].Needs = []string{"3"}
	if state := m.needState(m.notes[0]); state != "waits #3" {
		t.Fatalf("state = %q", state)
	}
	m.notes[2].Status = "done"
	if state := m.needState(m.notes[0]); state != "ready" {
		t.Fatalf("state = %q, want ready", state)
	}
	m.notes[0].Status = "doing"
	if state := m.needState(m.notes[0]); state != "" {
		t.Fatalf("running card shows %q", state)
	}
}

func TestBoardPriorityAndDependencyKeys(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const scope = "kanban"
	dep, err := tasks.Add(scope, "Dependency", nil)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := tasks.Add(scope, "Waiting", nil, tasks.MutationOptions{Needs: []string{dep.ID}})
	if err != nil {
		t.Fatal(err)
	}
	m := boardTestModel()
	m.notesScope = scope
	m.notes = tasks.List(scope)
	m.boardConfig = tasks.BoardConfig{Columns: tasks.DefaultColumns()}
	m.repoDir = t.TempDir()
	m.boardDir = m.repoDir
	m.taskCursor = slices.IndexFunc(m.notes, func(x tasks.Task) bool { return x.ID == waiting.ID })

	_, cmd := boardPress(m, "p")
	if cmd == nil {
		t.Fatal("p did not change the priority")
	}
	cmd()
	if got, _ := tasks.Get(scope, waiting.ID); got.Priority != "high" {
		t.Fatalf("priority = %q", got.Priority)
	}
	m.notes = tasks.List(scope)
	_, cmd = boardPress(m, "p")
	cmd()
	if got, _ := tasks.Get(scope, waiting.ID); got.Priority != "low" {
		t.Fatalf("priority = %q, want low", got.Priority)
	}
}

func TestBoardDependencyPicker(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const scope = "kanban"
	dep, err := tasks.Add(scope, "Dependency", nil)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := tasks.Add(scope, "Waiting", nil, tasks.MutationOptions{Needs: []string{dep.ID}})
	if err != nil {
		t.Fatal(err)
	}
	m := boardTestModel()
	m.notesScope = scope
	m.notes = tasks.List(scope)
	m.boardConfig = tasks.BoardConfig{Columns: tasks.DefaultColumns()}
	m.repoDir = t.TempDir()
	m.boardDir = m.repoDir
	m.taskCursor = slices.IndexFunc(m.notes, func(x tasks.Task) bool { return x.ID == waiting.ID })

	m, _ = boardPress(m, "D")
	if m.taskForm != "needs" || !strings.Contains(m.taskInput, dep.ID) {
		t.Fatalf("form=%q input=%q", m.taskForm, m.taskInput)
	}
	if i := slices.IndexFunc(m.needCandidates(), func(x tasks.Task) bool { return x.ID == dep.ID }); i >= 0 {
		m.taskStep = i
	} else {
		t.Fatal("dependency missing from the picker")
	}
	m, _ = boardPress(m, " ")
	m, cmd := boardPress(m, "enter")
	if m.taskForm != "" || cmd == nil {
		t.Fatalf("enter did not save: form=%q", m.taskForm)
	}
	cmd()
	if got, _ := tasks.Get(scope, waiting.ID); len(got.Needs) != 0 {
		t.Fatalf("dependencies not cleared: %+v", got.Needs)
	}
}
