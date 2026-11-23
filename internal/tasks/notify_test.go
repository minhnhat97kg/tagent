package tasks

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"tagent/internal/tmux"
	"testing"
)

func TestStatusNotifications(t *testing.T) {
	_, scope := testBoard(t)
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	var messages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := withLock(scope, func() error { return nil }); err != nil {
			t.Error(err)
		}
		b, _ := io.ReadAll(r.Body)
		messages = append(messages, string(b))
		w.WriteHeader(http.StatusBadGateway) // failure must not affect persistence
	}))
	defer srv.Close()
	fake.Set(srv.URL, "show", "-gv", "@agent_notify_url")
	ticket, err := Add(scope, strings.Repeat("界", 5000), []string{"tester"})
	if err != nil {
		t.Fatal(err)
	}
	check := func(err error, count int) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) != count {
			t.Fatalf("messages=%d want %d", len(messages), count)
		}
	}
	check(Move(scope, ticket.ID, "doing", ""), 0) // intermediate steps stay quiet
	check(Move(scope, ticket.ID, "doing", "comment"), 0)
	_, err = Update(scope, ticket.ID, "", []string{"builder"})
	check(err, 0)
	_, err = Update(scope, ticket.ID, "done", []string{"tester"})
	check(err, 1)
	if !strings.Contains(messages[0], ticket.ID) || !strings.Contains(messages[0], "#"+ticket.ID+" done") || !strings.Contains(messages[0], "comment") || !strings.Contains(messages[0], "tester") || len([]rune(messages[0])) > 4096 {
		t.Fatal(messages[0])
	}
	history, _ := ReadEvents(scope, 0)
	var seq int64
	for _, ev := range history {
		if ev.Kind == "status" {
			seq = ev.Seq
		}
	}
	check(FinishEntry(scope, ticket.ID, "todo", "stale", seq-1, false), 1)
	check(FinishEntry(scope, ticket.ID, "todo", "safety", seq, true), 1)
	if Move(scope, ticket.ID, "invalid", "") == nil {
		t.Fatal("accepted invalid status")
	}
	_, err = Update(scope, ticket.ID, "blocked", nil)
	check(err, 2)
	fake.Set("", "show", "-gv", "@agent_notify_url")
	_, err = Update(scope, ticket.ID, "done", nil)
	check(err, 2)
}

func TestBlockedNoticeAsksAndSaysHowToAnswer(t *testing.T) {
	_, scope := testBoard(t)
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	fake.Set(srv.URL, "show", "-gv", "@agent_notify_url")
	ticket, _ := Add(scope, "Fix login", nil)
	if err := Move(scope, ticket.ID, "doing", ""); err != nil {
		t.Fatal(err)
	}
	if err := Move(scope, ticket.ID, "blocked", "which API key should I use?"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"#" + ticket.ID + " blocked at doing", "Fix login", "which API key should I use?", "task comment " + ticket.ID} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %q", want, body)
		}
	}
}
