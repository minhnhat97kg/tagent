package web

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
	"tmux-agent/internal/intake"
	"tmux-agent/internal/tasks"
	"tmux-agent/internal/tmux"
)

func stringsReader(s string) io.Reader { return strings.NewReader(s) }
func testChatServer(t *testing.T) *server {
	t.Helper()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	return &server{token: "secret", dir: t.TempDir(), deps: apiDeps{sessions: func() []tmux.SessionInfo { return nil }, command: func() string { return "stub" }}}
}
func TestChatPOSTRunner(t *testing.T) {
	s := testChatServer(t)
	called := make(chan string, 1)
	release := make(chan struct{})
	s.deps.run = func(c, p string) (string, error) { called <- c + " " + p; <-release; return "ANSWER: hello", nil }
	w := request(s, "POST", "/api/chat", `{"op":"send","text":"hello"}`, true)
	if w.Code != 202 || !strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case prompt := <-called:
		if !strings.Contains(prompt, "hello") || !strings.HasPrefix(prompt, "stub ") {
			t.Fatal(prompt)
		}
	case <-time.After(time.Second):
		t.Fatal("runner not called")
	}
	c, _ := intake.LoadConversation(tasks.Scope(s.dir))
	if c.Status != "running" {
		t.Fatal(c)
	}
	close(release)
	for i := 0; i < 100; i++ {
		c, _ = intake.LoadConversation(tasks.Scope(s.dir))
		if c.Status == "idle" {
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	w = request(s, "GET", "/api/chat", "", true)
	if !strings.Contains(w.Body.String(), `"answer":"hello"`) || strings.Contains(w.Body.String(), `"Owner"`) {
		t.Fatal(w.Body.String())
	}
}
func TestChatDisabledAndValidation(t *testing.T) {
	s := testChatServer(t)
	s.deps.command = func() string { return "off" }
	if w := request(s, "POST", "/api/chat", `{"op":"send","text":"hello"}`, true); w.Code != 403 {
		t.Fatal(w.Code)
	}
	s.deps.command = func() string { return "stub" }
	for _, body := range []string{`{`, `{"op":"send","text":" "}`, `{"op":"create"}`, `{"op":"nonsense","revision":1}`} {
		w := request(s, "POST", "/api/chat", body, true)
		if w.Code != 400 && w.Code != 409 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
func TestChatProposalOnce(t *testing.T) {
	s := testChatServer(t)
	scope := tasks.Scope(s.dir)
	c, e := intake.ReserveSubmission(scope, "ticket", "")
	if e != nil {
		t.Fatal(e)
	}
	c, e = intake.CompleteSubmission(scope, c.Generation, "TICKET\ntitle: Test\ngoal: Test goal\n", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(tasks.List(scope)) != 0 {
		t.Fatal("premature task")
	}
	body := fmt.Sprintf(`{"op":"create","revision":%d}`, c.Revision)
	w := request(s, "POST", "/api/chat", body, true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(tasks.List(scope)) != 1 {
		t.Fatal("missing task")
	}
	w = request(s, "POST", "/api/chat", body, true)
	if w.Code != 409 || len(tasks.List(scope)) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	var dto map[string]any
	if json.Unmarshal(w.Body.Bytes(), &dto) == nil {
		t.Fatal("expected plain error")
	}
}

func TestChatCommandApprovalAndEdit(t *testing.T) {
	s := testChatServer(t)
	scope := tasks.Scope(s.dir)
	c, _ := intake.ReserveSubmission(scope, "!reload", "")
	c, e := intake.CompleteSubmission(scope, c.Generation, "CMD: tmux-agent reload", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	w := request(s, "POST", "/api/chat", fmt.Sprintf(`{"op":"edit","revision":%d,"command":"tmux-agent reload; bad"}`, c.Revision), true)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = request(s, "POST", "/api/chat", fmt.Sprintf(`{"op":"create","revision":%d}`, c.Revision), true)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	executed := make(chan string, 1)
	s.deps.execute = func(command, dir string) (string, error) { executed <- command + " " + dir; return "done", nil }
	w = request(s, "POST", "/api/chat", fmt.Sprintf(`{"op":"approve","revision":%d}`, c.Revision), true)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case value := <-executed:
		if value != "tmux-agent reload "+s.dir {
			t.Fatal(value)
		}
	case <-time.After(time.Second):
		t.Fatal("not executed")
	}
	for i := 0; i < 100; i++ {
		c, _ = intake.LoadConversation(scope)
		if c.Status == "idle" {
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	if c.Status != "idle" {
		t.Fatal(c)
	}
}
func TestChatCancelAndScope(t *testing.T) {
	s := testChatServer(t)
	scope := tasks.Scope(s.dir)
	c, _ := intake.ReserveSubmission(scope, "draft", "")
	c, _ = intake.CompleteSubmission(scope, c.Generation, "TICKET\ntitle: Test\ngoal: Test", nil, nil)
	other := t.TempDir()
	w := request(s, "GET", "/api/chat?dir="+other, "", true)
	if strings.Contains(w.Body.String(), `"title":"Test"`) {
		t.Fatal("scope leak")
	}
	w = request(s, "POST", "/api/chat", fmt.Sprintf(`{"op":"cancel","revision":%d}`, c.Revision), true)
	if w.Code != 200 || len(tasks.List(scope)) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	s.deps.command = func() string { return "off" }
	w = request(s, "GET", "/api/chat", "", true)
	if !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatal(w.Body.String())
	}
}
