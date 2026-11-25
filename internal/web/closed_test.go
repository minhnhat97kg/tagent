package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func TestCloseAndPurgeFromWeb(t *testing.T) {
	defer tmux.SetRunner(tmux.NewFake())()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	task, err := tasks.Add(scope, "Drop me", nil)
	if err != nil {
		t.Fatal(err)
	}
	s := server{dir: dir}
	w := httptest.NewRecorder()
	s.taskOp(w, httptest.NewRequest("POST", "/api/tasks", strings.NewReader(`{"Op":"set","ID":"`+task.ID+`","Status":"closed"}`)))
	if w.Code != 200 {
		t.Fatalf("close: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.taskOp(w, httptest.NewRequest("POST", "/api/tasks", strings.NewReader(`{"Op":"purge"}`)))
	var items []tasks.PurgeItem
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &items) != nil || len(items) != 1 || !items[0].Removed {
		t.Fatalf("purge: %d %s", w.Code, w.Body.String())
	}
	if _, err := tasks.Get(scope, task.ID); err == nil {
		t.Fatal("closed ticket kept")
	}
}
