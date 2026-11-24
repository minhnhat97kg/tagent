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

func TestTicketPriorityAndNeedsAPI(t *testing.T) {
	defer tmux.SetRunner(tmux.NewFake())()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	dep, err := tasks.Add(scope, "Dependency", nil)
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := tasks.Add(scope, "Waiting", nil)
	if err != nil {
		t.Fatal(err)
	}
	s := server{dir: dir}
	call := func(body string, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		s.taskOp(w, httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	call(`{"Op":"priority","ID":"`+waiting.ID+`","Priority":"high"}`, 200)
	call(`{"Op":"priority","ID":"`+waiting.ID+`","Priority":"urgent"}`, 400)
	call(`{"Op":"needs","ID":"`+waiting.ID+`","Needs":["`+dep.ID+`"]}`, 200)
	call(`{"Op":"needs","ID":"`+waiting.ID+`","Needs":["`+dep.ID+`","`+dep.ID+`"]}`, 400)
	call(`{"Op":"needs","ID":"`+dep.ID+`","Needs":["`+waiting.ID+`"]}`, 400) // cycle
	call(`{"Op":"set","ID":"`+waiting.ID+`","Status":"done"}`, 400)          // waiting for a dependency
	got, _ := tasks.Get(scope, waiting.ID)
	if got.Priority != "high" || len(got.Needs) != 1 || got.Needs[0] != dep.ID || got.Status != "todo" {
		t.Fatalf("ticket: %+v", got)
	}
	w := httptest.NewRecorder()
	s.project(w, httptest.NewRequest("GET", "/api/project", nil))
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, `"priority":"high"`) || !strings.Contains(body, `"needs":["`+dep.ID+`"]`) {
		t.Fatalf("project: %d %s", w.Code, body)
	}
	if _, err = tasks.Update(scope, dep.ID, "done", nil); err != nil {
		t.Fatal(err)
	}
	call(`{"Op":"needs","ID":"`+waiting.ID+`","Needs":[]}`, 200) // clearing is allowed
	if got, _ = tasks.Get(scope, waiting.ID); len(got.Needs) != 0 {
		t.Fatalf("needs not cleared: %+v", got)
	}
}

func TestTicketWorkflowAPI(t *testing.T) {
	defer tmux.SetRunner(tmux.NewFake())()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".tasks", "workflows"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tasks", "workflows", "single.yml"), []byte("name: single\njobs:\n  build: {run: echo ok}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "Typed", nil)
	if err != nil {
		t.Fatal(err)
	}
	s := server{dir: dir}
	call := func(body string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		s.taskOp(w, httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	call(`{"Op":"workflow","ID":"`+ticket.ID+`","Workflow":"nope"}`, 400)
	call(`{"Op":"workflow","ID":"`+ticket.ID+`","Workflow":"single"}`, 200)
	if got, _ := tasks.Get(scope, ticket.ID); got.Workflow != "single" {
		t.Fatalf("not set: %+v", got)
	}
	call(`{"Op":"workflow","ID":"`+ticket.ID+`","Workflow":""}`, 200)
	if got, _ := tasks.Get(scope, ticket.ID); got.Workflow != "" {
		t.Fatalf("not cleared: %+v", got)
	}
	call(`{"Op":"set","ID":"`+ticket.ID+`","Status":"blocked"}`, 200)
	call(`{"Op":"workflow","ID":"`+ticket.ID+`","Workflow":"single"}`, 400) // not todo
}
