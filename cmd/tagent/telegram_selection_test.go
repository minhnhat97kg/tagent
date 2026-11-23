package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"tagent/internal/board"
	"tagent/internal/intake"
	"tagent/internal/notify"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func TestTelegramSelectionRetriesPendingSend(t *testing.T) {
	for _, reply := range []string{"Q: Which size?", "CMD: tagent reload"} {
		t.Run(reply, func(t *testing.T) {
			t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			dir := t.TempDir()
			if err := board.Register(dir); err != nil {
				t.Fatal(err)
			}
			scope := tasks.Scope(dir)
			for _, raw := range []string{"ANSWER: old history", reply} {
				s, err := intake.ReserveSubmission(scope, "request", "")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = intake.CompleteSubmission(scope, s.Generation, raw, nil, nil); err != nil {
					t.Fatal(err)
				}
			}
			state, err := intake.LoadConversation(scope)
			if err != nil {
				t.Fatal(err)
			}
			var replies []string
			fail := true
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if fail {
					http.Error(w, "retry", http.StatusInternalServerError)
					return
				}
				r.ParseForm()
				replies = append(replies, r.Form.Get("text"))
				fmt.Fprint(w, `{"ok":true,"result":[]}`)
			}))
			defer srv.Close()
			raw := "https://api.telegram.org/botsecret/sendMessage?chat_id=9"
			runner := &telegramRunner{opts: map[string]string{"@agent_scrape_pid": strconv.Itoa(os.Getpid()), "@agent_notify_url": raw}}
			defer tmux.SetRunner(runner)()
			c := telegramControl{ctx: context.Background(), client: srv.Client(), raw: raw, base: srv.URL + "/", chat: "9", token: "secret"}
			var u notify.TelegramUpdate
			u.ID = 42
			u.Message.Chat.ID = 9
			u.Message.Chat.Type = "private"
			u.Message.Text = "/chat 1"
			c.updates([]notify.TelegramUpdate{u})
			cursor, err := intake.DeliveryCursor(scope, c.endpoint(), nil)
			if err != nil || cursor != 0 || runner.opts["@agent_telegram_offset"] != "" || runner.opts[telegramProject] != "" {
				t.Fatal("failed selection was checkpointed", cursor, err, runner.opts)
			}
			fail = false
			c.updates([]notify.TelegramUpdate{u}) // same Telegram update must retry
			cursor, err = intake.DeliveryCursor(scope, c.endpoint(), nil)
			if err != nil || cursor != state.Revision || runner.opts["@agent_telegram_offset"] != "43" || runner.opts[telegramProject] != dir {
				t.Fatal(cursor, err, runner.opts)
			}
			if len(replies) != 2 || !strings.Contains(replies[0], strings.TrimPrefix(reply, "Q: ")) || strings.Contains(strings.Join(replies, "\n"), "old history") {
				t.Fatal(replies)
			}
			c.completion()
			c.updates([]notify.TelegramUpdate{u})
			if len(replies) != 2 {
				t.Fatal("successful selection replayed", replies)
			}
		})
	}
}
