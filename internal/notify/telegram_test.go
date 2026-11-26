package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"

	"tagent/internal/tmux"
)

type sentCall struct {
	Method string
	Params map[string]any
}

type rig struct {
	mu     sync.Mutex
	http   []sentCall
	agents [][]string
	ups    []update
}

// setupRig points the gateway at a fake Bot API, a fake agent CLI and a fixed
// pane list. Updates are served by the first getUpdates call.
func setupRig(t *testing.T, token string, ups ...update) *rig {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	r := &rig{ups: ups}

	fake := tmux.NewFake()
	fake.Set(token, "show", "-gv", tokenOption)
	fake.Set("100,-200", "show", "-gv", chatsOption)
	t.Cleanup(tmux.SetRunner(fake))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var params map[string]any
		_ = json.NewDecoder(req.Body).Decode(&params)
		method := path.Base(req.URL.Path)
		r.mu.Lock()
		r.http = append(r.http, sentCall{method, params})
		served := r.ups
		if method == "getUpdates" {
			r.ups = nil
		}
		r.mu.Unlock()
		resp := `{"ok":true,"result":true}`
		switch method {
		case "getUpdates":
			b, _ := json.Marshal(served)
			resp = `{"ok":true,"result":` + string(b) + `}`
		case "sendMessage":
			resp = `{"ok":true,"result":{"message_id":1001}}`
		}
		io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)

	oldAPI, oldRun, oldList, oldBg, oldDetach := telegramAPI, runTagent, listAgents, background, detachAlert
	telegramAPI = srv.URL
	detachAlert = func(pane, title, body string) { SendTelegramAlert(pane, title, body) }
	t.Cleanup(func() {
		telegramAPI, runTagent, listAgents, background, detachAlert = oldAPI, oldRun, oldList, oldBg, oldDetach
	})
	runTagent = func(_ context.Context, args ...string) (string, string, int) {
		r.mu.Lock()
		r.agents = append(r.agents, args)
		r.mu.Unlock()
		switch {
		case args[0] == "answer" && args[len(args)-1] == "?":
			return "Allow the edit?\n  1  Yes\n  2  No\n", "", 0
		case args[0] == "send":
			return "queued\nidle\n", "", 0
		}
		return "", "", 0
	}
	listAgents = func() []agentPane {
		return []agentPane{
			{ID: "%3", Name: "dev", Agent: "claude", Status: tmux.StatusIdle, PID: "300"},
			{ID: "%5", Name: "rec", Agent: "claude", Status: tmux.StatusIdle, PID: "500"},
			{ID: "%7", Name: "wait", Agent: "claude", Status: tmux.StatusWaiting, PID: "700"},
		}
	}
	background = func(f func()) { f() } // run slow work inline so the test can assert on it
	return r
}

func (r *rig) agentCalled(args ...string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.agents {
		if strings.Join(c, "\x00") == strings.Join(args, "\x00") {
			return true
		}
	}
	return false
}

// sentTo reports whether any Bot API call of method mentions text (or the
// chat when chat != "") in its params.
func (r *rig) sentTo(method, chat, text string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.http {
		if c.Method != method {
			continue
		}
		b, _ := json.Marshal(c.Params)
		s := string(b)
		if (chat == "" || fmt.Sprint(c.Params["chat_id"]) == chat) && strings.Contains(s, text) {
			return true
		}
	}
	return false
}

func msg(id, chat int64, text string, replyTo int64) update {
	u := update{ID: id, Message: &message{ID: id, Chat: chatInfo{ID: chat}, Text: text}}
	if replyTo != 0 {
		u.Message.ReplyTo = &message{ID: replyTo}
	}
	return u
}

func tap(id, chat int64, data string) update {
	return update{ID: id, Callback: &callback{ID: fmt.Sprintf("cb%d", id), Data: data, Message: &message{Chat: chatInfo{ID: chat}}}}
}

func TestTelegramGateway(t *testing.T) {
	t.Run("routing and allowlist", func(t *testing.T) {
		r := setupRig(t, "tok-secret",
			msg(1, 100, "hello from reply", 7),
			msg(2, 100, "@rec hi there", 0),
			msg(3, 100, "/use dev", 0),
			msg(4, 100, "plain after use", 0),
			msg(5, 999, "intruder", 0),
			tap(6, 999, "a|%3|1|x"),
			tap(7, 100, "a|%7|1|stale"),
			msg(8, 100, "/clear", 0),
		)
		rememberMessage(100, 7, agentPane{ID: "%3", PID: "300"})
		if err := pollOnce(loadConfig(), nil); err != nil {
			t.Fatal(err)
		}

		want := func(pane, text string) []string { return []string{"send", pane, "--timeout", "10m", text} }
		if !r.agentCalled(want("%3", "hello from reply")...) {
			t.Error("reply-to did not route to the mapped pane")
		}
		if !r.agentCalled(want("%5", "hi there")...) {
			t.Error("@name did not route to that agent")
		}
		if !r.agentCalled(want("%3", "plain after use")...) {
			t.Error("plain text after /use did not route to the chat target")
		}
		if r.sentTo("sendMessage", "999", "") || r.agentCalled("answer", "%3", "x") || r.sentTo("answerCallbackQuery", "", "cb6") {
			t.Error("non-allowlisted chat was acted on")
		}
		for _, c := range r.agents {
			if strings.Contains(strings.Join(c, " "), "intruder") {
				t.Error("non-allowlisted message was forwarded")
			}
		}
		if r.agentCalled(want("%3", "/clear")...) || !r.sentTo("sendMessage", "100", "not forwarded") {
			t.Error("slash text was forwarded instead of refused")
		}
	})

	t.Run("dialog tap needs the alerted screen", func(t *testing.T) {
		r := setupRig(t, "tok-secret")
		h := dialogHash("%7", []dialogOpt{{"1", "Yes"}, {"2", "No"}})
		r.ups = []update{
			tap(30, 100, "a|%7|1|nothash"), // stale: refused, nothing typed
			tap(31, 100, "a|%7|2|"+h),      // same screen as alerted: answered
		}
		if err := pollOnce(loadConfig(), nil); err != nil {
			t.Fatal(err)
		}
		if r.agentCalled("answer", "%7", "1") || !r.sentTo("answerCallbackQuery", "", "dialog changed") {
			t.Error("stale-hash tap was not refused")
		}
		if !r.agentCalled("answer", "%7", "2") || !r.sentTo("answerCallbackQuery", "", "sent 2") {
			t.Error("tap on the alerted screen was not answered")
		}
	})

	t.Run("agents list, use button and offset", func(t *testing.T) {
		r := setupRig(t, "tok-secret",
			msg(20, 100, "/agents", 0),
			tap(21, 100, "u|rec"),
			msg(22, 100, "after tap", 0),
		)
		if err := pollOnce(loadConfig(), nil); err != nil {
			t.Fatal(err)
		}
		if !r.sentTo("sendMessage", "100", `"callback_data":"u|rec"`) {
			t.Error("/agents did not offer a /use button per agent")
		}
		if !r.agentCalled("send", "%5", "--timeout", "10m", "after tap") {
			t.Error("the u| tap did not set the chat target")
		}
		if got := loadState().Offset; got != 23 {
			t.Errorf("offset %d, want 23 (persisted before handling)", got)
		}
	})

	t.Run("send and alert reach allowlisted chats only", func(t *testing.T) {
		r := setupRig(t, "tok-secret")
		Send("ping", "body")
		if !r.sentTo("sendMessage", "100", "ping") || !r.sentTo("sendMessage", "-200", "ping") || r.sentTo("sendMessage", "999", "") {
			t.Error("Send did not reach exactly the allowlisted chats")
		}
	})

	t.Run("empty token is off", func(t *testing.T) {
		r := setupRig(t, "")
		Send("ping", "body", "%3")
		if len(r.http) != 0 || len(r.agents) != 0 {
			t.Errorf("empty token made calls: http %d, agent %d", len(r.http), len(r.agents))
		}
	})

	t.Run("transport error hides the token", func(t *testing.T) {
		setupRig(t, "tok-secret")
		telegramAPI = "http://127.0.0.1:1"
		err := tgCall(context.Background(), "tok-secret", "getUpdates", map[string]any{}, nil)
		if err == nil || strings.Contains(err.Error(), "tok-secret") {
			t.Errorf("expected an error without the token, got %v", err)
		}
	})
}
