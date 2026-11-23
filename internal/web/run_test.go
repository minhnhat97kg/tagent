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

func TestRunView(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	path := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "sample.yml"), []byte("name: sample\njobs:\n  plan: {agent: planner}\n  build: {needs: plan, agent: implementer}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	task, err := tasks.Add(scope, "Run view", nil, tasks.MutationOptions{Workflow: "sample"})
	if err != nil {
		t.Fatal(err)
	}
	if err = tasks.Move(scope, task.ID, "plan", "start"); err != nil {
		t.Fatal(err)
	}
	s := &server{dir: dir}
	call := func(id string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "/api/tasks/"+id+"/run?dir="+dir, nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		s.run(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", id, w.Code, w.Body.String())
		}
		return w
	}
	w := call(task.ID, 200)
	// A current entry has no end; a zero time would surface as a year-1 date.
	if strings.Contains(w.Body.String(), `"end"`) {
		t.Fatalf("current entry carries an end: %s", w.Body.String())
	}
	var v runView
	if err = json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Ticket != task.ID || v.Workflow != "sample" || v.Status != "plan" || v.Title != "Run view" {
		t.Fatalf("header: %+v", v)
	}
	if len(v.Entries) != 1 || v.Entries[0].Step != "plan" || v.Entries[0].Result != "current" || v.Entries[0].Loop != 1 {
		t.Fatalf("entries: %+v", v.Entries)
	}
	if len(v.Flow) != 2 || v.Flow[0].Job != "plan" || v.Flow[0].Count != 1 || !v.Flow[0].Current || v.Flow[1].Count != 0 {
		t.Fatalf("flow: %+v", v.Flow)
	}
	plain, err := tasks.Add(scope, "No workflow", nil)
	if err != nil {
		t.Fatal(err)
	}
	call(plain.ID, 404)
	call("missing", 404)
}
