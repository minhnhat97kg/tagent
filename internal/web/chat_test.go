package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"tagent/internal/intake"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"testing"
	"time"
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
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		called <- string(b)
		<-release
		fmt.Fprint(w, `{"message":{"tool_calls":[{"function":{"name":"answer","arguments":{"text":"hello"}}}]}}`)
	}))
	defer ts.Close()
	s.deps.harness = &intake.Harness{Endpoint: ts.URL, Model: "stub"}
	w := request(s, "POST", "/api/chat", `{"op":"send","text":"hello"}`, true)
	if w.Code != 202 || !strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case prompt := <-called:
		if !strings.Contains(prompt, "hello") || !strings.Contains(prompt, `"model":"stub"`) {
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
func TestChatMentionApproval(t *testing.T) {
	s := testChatServer(t)
	fake := tmux.NewFake()
	fake.Set("builder", "show", "-t", "%1", "-pv", "@agent_role")
	restore := tmux.SetRunner(fake)
	defer restore()
	s.deps.sessions = func() []tmux.SessionInfo {
		return []tmux.SessionInfo{{Windows: []tmux.WindowInfo{{Panes: []tmux.PaneInfo{{PaneID: "%1", Name: "one", Path: s.dir, PanePID: 42}}}}}}
	}
	defer intake.SetLiveSessions(s.deps.sessions)()
	done := make(chan []string, 1)
	s.deps.toolExecute = func(_ context.Context, dir, binary string, args []string) (string, error) {
		if dir != s.dir || binary != "tagent" {
			t.Error("wrong execution scope")
		}
		done <- args
		return "queued", nil
	}
	w := request(s, "POST", "/api/chat", `{"op":"send","text":"@one hello"}`, true)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	c, e := intake.LoadConversation(tasks.Scope(s.dir))
	if e != nil || c.Status != "proposal" {
		t.Fatal(c, e)
	}
	select {
	case <-done:
		t.Fatal("sent before approval")
	default:
	}
	w = request(s, "GET", "/api/chat", "", true)
	if !strings.Contains(w.Body.String(), `"Role":"builder"`) || !strings.Contains(w.Body.String(), `"send_prompt"`) {
		t.Fatal(w.Body.String())
	}
	w = request(s, "POST", "/api/chat", fmt.Sprintf(`{"op":"approve","revision":%d}`, c.Revision), true)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case args := <-done:
		if strings.Join(args, "|") != "send|%1|hello" {
			t.Fatal(args)
		}
	case <-time.After(time.Second):
		t.Fatal("not routed")
	}
	w = request(s, "POST", "/api/chat", fmt.Sprintf(`{"op":"approve","revision":%d}`, c.Revision), true)
	if w.Code != 409 {
		t.Fatal("stale approval accepted", w.Code)
	}
	for i := 0; i < 100; i++ {
		c, _ = intake.LoadConversation(tasks.Scope(s.dir))
		if c.Status == "idle" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("approval did not finish")
}

func TestChatClear(t *testing.T) {
	s := testChatServer(t)
	scope := tasks.Scope(s.dir)
	c, err := intake.ReserveSubmission(scope, "old", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{`"op":"clear"`, `"op":"send","text":"/clear"`} {
		w := request(s, "POST", "/api/chat", fmt.Sprintf(`{%s,"revision":%d}`, op, c.Revision), true)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		c, err = intake.LoadConversation(scope)
		if err != nil || len(c.Messages) != 0 || c.Status != "idle" {
			t.Fatal(c, err)
		}
	}
	w := request(s, "POST", "/api/chat", `{"op":"clear","revision":1}`, true)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
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

func TestModelNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Write([]byte(`{"models":[{"name":"qwen3:4b"},{"name":"bonsai-2:9b"}]}`))
	}))
	defer srv.Close()
	got := modelNames(context.Background(), intake.Harness{Endpoint: srv.URL})
	if len(got) != 2 || got[0] != "bonsai-2:9b" || got[1] != "qwen3:4b" {
		t.Fatalf("got %v", got)
	}
	if modelNames(context.Background(), intake.Harness{Endpoint: "http://127.0.0.1:1"}) != nil {
		t.Fatal("unreachable endpoint should give none")
	}
}

func TestOpenAIModelNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("%s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Write([]byte(`{"data":[{"id":"gpt-b"},{"id":"gpt-a"}]}`))
	}))
	defer srv.Close()
	got := modelNames(context.Background(), intake.Harness{Endpoint: srv.URL + "/v1", OpenAI: true, APIKey: "k"})
	if len(got) != 2 || got[0] != "gpt-a" {
		t.Fatalf("got %v", got)
	}
}
