package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowCRUD(t *testing.T) {
	s := &server{dir: t.TempDir()}
	call := func(method, body string, want int) {
		t.Helper()
		r := httptest.NewRequest(method, "/api/workflows", strings.NewReader(body))
		w := httptest.NewRecorder()
		s.workflows(w, r)
		if w.Code != want {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	call("POST", `{"Name":"build","Op":"new"}`, 200)
	call("GET", ``, 200)
	call("POST", `{"Name":"build","YAML":"jobs: {test: {run: go test ./...}}"}`, 200)
	call("POST", `{"Name":"build","YAML":"jobs: {}"}`, 400)
	call("DELETE", `{"Name":"build"}`, 200)
	for _, name := range []string{"../escape", "/tmp/escape", "a/b", ".."} {
		if _, err := workflowPath(s.dir, name); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	for _, method := range []string{"POST", "DELETE"} {
		call(method, `{"Name":"global","Global":true}`, 400)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(s.dir, ".tasks", "workflows", "escape.yaml")); err != nil {
		t.Fatal(err)
	}
	call("POST", `{"Name":"escape","Op":"new"}`, 400)
}

func TestAgentActionRejectsNonAgent(t *testing.T) {
	s := &server{}
	w := httptest.NewRecorder()
	s.agentAction(w, httptest.NewRequest(http.MethodPost, "/api/pane/action", strings.NewReader(`{"pane":"shell","data":"undo"}`)))
	if w.Code != 400 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestWorkflowValidateReturnsLine(t *testing.T) {
	s := &server{dir: t.TempDir()}
	r := httptest.NewRequest("POST", "/api/workflows", strings.NewReader(`{"Op":"validate","YAML":"jobs:\n  a: {run: x}\n  b: {run: y, needs: z}\n"}`))
	w := httptest.NewRecorder()
	s.workflows(w, r)
	if w.Code != 400 || !strings.Contains(w.Body.String(), `"line":3`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
