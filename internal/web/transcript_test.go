package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/queue"
	"tagent/internal/tmux"
)

func TestTranscriptAPI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	path := filepath.Join(home, ".claude", "projects", "p", "s1.jsonl")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte(`{"type":"user","uuid":"u","message":{"role":"user","content":"ask"}}`+"\n"+
		`{"type":"assistant","uuid":"a","parentUuid":"u","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"go test ./..."}}]}}`+"\n"), 0600)
	fake := tmux.NewFake()
	fake.Set("claude", "show", "-t", "%3", "-pv", tmux.PaneAgent)
	fake.Set("s1", "show", "-t", "%3", "-pv", tmux.PaneSessionID)
	fake.Set("running", "show", "-t", "%3", "-pv", tmux.PaneStatusOpt)
	defer tmux.SetRunner(fake)()
	s := &server{token: "secret", dir: t.TempDir()}

	// The pane id is path-escaped by the browser (%3 -> %253).
	w := request(s, "GET", "/api/agents/%253/transcript", "", true)
	var got struct {
		Items  []struct{ Role, Text string }
		Cursor string
		Status string
		Reset  bool
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err, w.Body.String())
	}
	if len(got.Items) != 2 || got.Items[0].Text != "ask" || got.Status != "running" {
		t.Fatalf("%s", w.Body)
	}
	// Incremental: nothing after the cursor.
	w = request(s, "GET", "/api/agents/%253/transcript?after="+got.Cursor, "", true)
	if !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal(w.Body.String())
	}

	// Busy agent: the message is queued, not typed.
	w = request(s, "POST", "/api/pane/send", `{"pane":"%3","data":"next step"}`, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"queued":true`) || len(queue.List("%3")) != 1 {
		t.Fatalf("send while running: %d %s", w.Code, w.Body)
	}
}
