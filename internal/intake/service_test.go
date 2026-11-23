package intake

import (
	"errors"
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
	_, e = RunSubmission(s, "stub", func(c, p string) (string, error) {
		if c != "stub" || !strings.Contains(p, "hello") {
			t.Fatal(c, p)
		}
		return "ANSWER: hello", nil
	})
	if e != nil {
		t.Fatal(e)
	}
	c, e := LoadConversation(scope)
	if e != nil || c.Status != "idle" || c.Response.Answer != "hello" {
		t.Fatal(c, e)
	}
	s, e = PrepareSubmission(dir, scope, "again", nil)
	if e != nil {
		t.Fatal(e)
	}
	RunSubmission(s, "stub", func(string, string) (string, error) { return "", errors.New("failure") })
	c, _ = LoadConversation(scope)
	if c.Status != "failed" {
		t.Fatal(c)
	}
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
