package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func TestKanbanMutations(t *testing.T) {
	defer tmux.SetRunner(tmux.NewFake())()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	task, err := tasks.Add(scope, "Kanban task", nil)
	if err != nil {
		t.Fatal(err)
	}
	s := server{dir: dir}
	for _, body := range []string{
		`{"Op":"set","ID":"` + task.ID + `","Status":"done"}`,
		`{"Op":"comment","ID":"` + task.ID + `","Text":"Reviewed"}`,
	} {
		w := httptest.NewRecorder()
		s.taskOp(w, httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body)))
		if w.Code != 200 {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
	}
	list := tasks.List(scope)
	if len(list) != 1 || list[0].Status != "done" || len(list[0].Comments) != 1 || list[0].Comments[0].Body != "Reviewed" {
		t.Fatalf("unexpected task: %+v", list)
	}
}

func TestKanbanStartWorkflow(t *testing.T) {
	defer tmux.SetRunner(tmux.NewFake())()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".tasks", "workflows"), 0o755)
	os.WriteFile(filepath.Join(dir, ".tasks", "workflows", "ship.yaml"), []byte("jobs:\n  build:\n    status: build\n    run: \"true\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".tasks", "board.yaml"), []byte("statuses:\n  - {id: todo}\n  - {id: build}\n  - {id: done}\n  - {id: blocked}\n"), 0o644)
	task, err := tasks.Add(tasks.Scope(dir), "Ship it", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	(&server{dir: dir}).taskOp(w, httptest.NewRequest("POST", "/api/tasks", strings.NewReader(`{"Op":"start","ID":"`+task.ID+`","Status":"ship"}`)))
	got, _ := tasks.Get(tasks.Scope(dir), task.ID)
	if w.Code != 200 || got.Workflow != "ship" || got.Status != "build" {
		t.Fatalf("%d %s: %+v", w.Code, w.Body.String(), got)
	}
}
