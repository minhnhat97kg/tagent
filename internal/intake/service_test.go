package intake

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"tagent/internal/tasks"
	"testing"
)

func TestSubmissionService(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	s, e := PrepareSubmission(dir, scope, "hello", nil)
	if e != nil {
		t.Fatal(e)
	}
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/chat") || !strings.Contains(readAll(r), "hello") {
			t.Error("prompt not sent")
		}
		w.Write([]byte(`{"message":{"tool_calls":[{"function":{"name":"answer","arguments":{"text":"hello"}}}]}}`))
	}))
	defer ok.Close()
	RunReceptionist(s, Harness{Endpoint: ok.URL, Model: "stub"})
	c, e := LoadConversation(scope)
	if e != nil || c.Status != "idle" || c.Response.Answer != "hello" {
		t.Fatal(c, e)
	}
	s, e = PrepareSubmission(dir, scope, "again", nil)
	if e != nil {
		t.Fatal(e)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", 503) }))
	defer down.Close()
	RunReceptionist(s, Harness{Endpoint: down.URL, Model: "stub"})
	c, _ = LoadConversation(scope)
	if c.Status != "failed" {
		t.Fatal(c)
	}
}
func readAll(r *http.Request) string {
	b, _ := io.ReadAll(r.Body)
	return string(b)
}
func TestCreateDraftWithoutWorkflow(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	out, e := CreateDraft(dir, scope, Draft{Title: "Test", Goal: "Cover"}, true)
	if e != nil || !strings.HasPrefix(out, "Created ") || len(tasks.List(scope)) != 1 {
		t.Fatal(out, e)
	}
}
