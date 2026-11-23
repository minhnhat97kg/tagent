package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"tagent/internal/board"
	"tagent/internal/intake"
	"tagent/internal/notify"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"testing"
	"time"
)

type telegramRunner struct {
	mu   sync.Mutex
	opts map[string]string
	fail bool
}

func (r *telegramRunner) Run(a ...string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a[0] == "show" {
		return r.opts[a[len(a)-1]], nil
	}
	if a[0] == "set" {
		if r.fail {
			return "", fmt.Errorf("failed")
		}
		r.opts[a[len(a)-2]] = a[len(a)-1]
		return "", nil
	}
	return "", nil
}
func TestTelegramSharedConversation(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	if e := board.Register(dir); e != nil {
		t.Fatal(e)
	}
	var mu sync.Mutex
	var replies []string
	failed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		mu.Lock()
		defer mu.Unlock()
		if failed {
			http.Error(w, "retry", 500)
			return
		}
		replies = append(replies, r.Form.Get("text"))
		fmt.Fprint(w, `{"ok":true,"result":[]}`)
	}))
	defer srv.Close()
	raw := "https://api.telegram.org/botsecret/sendMessage?chat_id=9"
	runner := &telegramRunner{opts: map[string]string{"@agent_scrape_pid": strconv.Itoa(os.Getpid()), "@agent_notify_url": raw}}
	defer tmux.SetRunner(runner)()
	c := telegramControl{ctx: context.Background(), client: srv.Client(), raw: raw, base: srv.URL + "/", chat: "9", token: "secret"}
	old := telegramRun
	defer func() { telegramRun = old }()
	prompts := make(chan string, 4)
	telegramRun = func(command, prompt string) (string, error) {
		prompts <- prompt
		if strings.Contains(prompt, "small") {
			return "CMD: tagent reload", nil
		}
		return "Q: How large?", nil
	}
	update := func(id int64, chat int64, kind, text string) {
		var u notify.TelegramUpdate
		u.ID = id
		u.Message.Chat.ID = chat
		u.Message.Chat.Type = kind
		u.Message.Text = text
		c.updates([]notify.TelegramUpdate{u})
	}
	update(1, 10, "private", "ignored")
	update(2, 9, "group", "ignored")
	if len(replies) != 0 {
		t.Fatal("unauthorized response")
	}
	update(3, 9, "private", "build a thing") // no /chat selection
	<-prompts
	scope := tasks.Scope(dir)
	wait := func(status string) intake.Conversation {
		t.Helper()
		for range 200 {
			s, e := intake.LoadConversation(scope)
			if e != nil {
				t.Fatal(e)
			}
			if s.Status == status {
				return s
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("request never completed")
		return intake.Conversation{}
	}
	s := wait("idle")
	if s.Response.Question != "How large?" || s.Idea != "build a thing" {
		t.Fatal(s)
	}
	c.completion()
	n := len(replies)
	c.completion()
	if len(replies) != n {
		t.Fatal("replayed delivery")
	}
	// The same API used by dashboard Chat answers a Telegram clarification.
	s, e := intake.ReserveSubmission(scope, "small", "")
	if e != nil {
		t.Fatal(e)
	}
	s, e = intake.CompleteSubmission(scope, s.Generation, "CMD: tagent reload", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	update(4, 9, "private", "/yes")
	unchanged := wait("proposal")
	if unchanged.Revision != s.Revision || !strings.Contains(replies[len(replies)-1], "local approval") {
		t.Fatal("remote command approval was not rejected")
	}
	// Only the local Chat approval path can claim and finish the proposal.
	s, e = intake.ClaimApproval(scope, s.Revision)
	if e != nil {
		t.Fatal(e)
	}
	_, e = intake.FinishApproval(scope, s.Generation, "done secret")
	if e != nil {
		t.Fatal(e)
	}
	c.completion()
	if strings.Contains(strings.Join(replies, "\n"), "done secret") {
		t.Fatal("token leak")
	}
	// Chat-origin question can be answered from Telegram.
	s, e = intake.ReserveSubmission(scope, "another idea", "")
	if e != nil {
		t.Fatal(e)
	}
	s, e = intake.CompleteSubmission(scope, s.Generation, "Q: How large?", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	update(5, 9, "private", "small")
	prompt := <-prompts
	if !strings.Contains(prompt, "another idea") || !strings.Contains(prompt, "small") {
		t.Fatal(prompt)
	}
	s = wait("proposal")
	if len(s.Turns) != 1 {
		t.Fatal(s)
	}
	mu.Lock()
	failed = true
	mu.Unlock()
	c.completion()
	cursor, _ := intake.DeliveryCursor(scope, c.endpoint(), nil)
	if cursor >= s.Revision {
		t.Fatal("failed send marked delivered")
	}
	mu.Lock()
	failed = false
	mu.Unlock()
	c.completion()
	update(6, 9, "private", "/no")
	s = wait("idle")
	if s.Response.Command != "" {
		t.Fatal(s)
	}
	update(7, 9, "private", "/read 0")
	if !strings.Contains(replies[len(replies)-1], "1-48") {
		t.Fatal(replies)
	}
	update(8, 9, "private", "/stop")
	if runner.opts[telegramProject] != "" {
		t.Fatal("not detached")
	}
	s, e = intake.ReserveSubmission(scope, "create a ticket", "")
	if e != nil {
		t.Fatal(e)
	}
	_, e = intake.CompleteSubmission(scope, s.Generation, "TICKET\ntitle: Test ticket\ngoal: Verify confirmation\nworkflow: none", nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	update(9, 9, "private", "ok")
	wait("proposal")
	if len(tasks.List(scope)) != 0 || !strings.Contains(replies[len(replies)-1], "local approval") {
		t.Fatal("remote draft approval was not rejected")
	}
	if !c.command("/no") {
		t.Fatal("cancellation failed")
	}
	telegramRun = func(string, string) (string, error) { return "", errors.New("model unavailable") }
	update(10, 9, "private", "another request")
	wait("failed")
	runner.opts["@agent_scrape_pid"] = "other"
	n = len(replies)
	update(11, 9, "private", "/help")
	if len(replies) != n {
		t.Fatal("lost owner sent reply")
	}
}
func TestTelegramHelpDocumented(t *testing.T) {
	for _, cmd := range []string{"/list", "/chat", "/read", "/stop", "/help"} {
		if !strings.Contains(helpText, cmd) || !strings.Contains(telegramHelp, cmd) {
			t.Fatal(cmd)
		}
	}
}
func TestTelegramPollTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ok":true,"result":[{"update_id":42}]}`) }))
	defer srv.Close()
	var u []notify.TelegramUpdate
	if !notify.TelegramCall(context.Background(), srv.Client(), srv.URL+"/", "getUpdates", nil, &u) || len(u) != 1 || u[0].ID != 42 {
		t.Fatal(u)
	}
}
