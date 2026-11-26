package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/tasks"
)

func TestTicketWorkplace(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	path := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	recipes := map[string]string{
		"folder": "workplace: folder\ncleanup: keep\njobs:\n  plan: {agent: planner}\n",
		"docker": "workplace: docker\njobs:\n  plan: {agent: planner}\n",
	}
	for name, body := range recipes {
		if err := os.WriteFile(filepath.Join(path, name+".yml"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	task, err := tasks.Add(scope, "Workplace", nil, tasks.MutationOptions{Workflow: "folder"})
	if err != nil {
		t.Fatal(err)
	}
	s := &server{dir: dir}
	call := func(id string, want int) map[string]any {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/tasks/"+id+"/workplace?dir="+dir, nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		s.workplace(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", id, w.Code, w.Body.String())
		}
		var v map[string]any
		if w.Code == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
				t.Fatal(err)
			}
		}
		return v
	}
	if v := call(task.ID, 200); v["type"] != "folder" || v["cleanup"] != "keep" || v["state"] != "not created" || !strings.HasSuffix(v["location"].(string), "folder/"+task.ID) {
		t.Fatalf("before create: %v", v)
	}
	wp, err := tasks.TicketWorkplace(scope, task)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wp.Create(tasks.MainRoot(dir), task.ID); err != nil {
		t.Fatal(err)
	}
	if v := call(task.ID, 200); v["state"] != "active" {
		t.Fatalf("after create: %v", v)
	}
	w := httptest.NewRecorder()
	s.taskOp(w, httptest.NewRequest("POST", "/api/tasks", strings.NewReader(`{"Op":"set","ID":"`+task.ID+`","Status":"done"}`)))
	if w.Code != 200 {
		t.Fatalf("done: %d %s", w.Code, w.Body.String())
	}
	if v := call(task.ID, 200); v["state"] != "kept" {
		t.Fatalf("done with keep: %v", v)
	}
	docker, err := tasks.Add(scope, "Docker", nil, tasks.MutationOptions{Workflow: "docker"})
	if err != nil {
		t.Fatal(err)
	}
	if v := call(docker.ID, 200); v["error"] == nil || v["type"] != nil {
		t.Fatalf("docker without image: %v", v)
	}
	call("missing", 404)

	w = httptest.NewRecorder()
	s.workflows(w, httptest.NewRequest("GET", "/api/workflows?dir="+dir, nil))
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row["name"] == "folder" {
			found = true
			if row["workplace"] != "folder" || row["cleanup"] != "keep" {
				t.Fatalf("workflow row: %v", row)
			}
		}
	}
	if !found {
		t.Fatalf("no folder workflow: %s", w.Body.String())
	}
}
