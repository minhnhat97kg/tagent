package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tagent/internal/quota"
	"tagent/internal/routing"
	"tagent/internal/tasks"
)

func routingWindowAt(used float64) quota.Window {
	return quota.Window{Used: &used, Reset: []byte(`"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"`)}
}

func TestRoutingAPI(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(tasks.BoardConfigPath(dir)), 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(tasks.BoardConfigPath(dir), []byte("roles: {dev: {tier: fast, effort: medium}}\n"), 0600)
	snap := quota.Snapshot{}
	s := &server{token: "secret", dir: dir, deps: apiDeps{quota: func() quota.Snapshot { return snap }}}
	url := "/api/routing?dir=" + dir
	view := func(w *httptest.ResponseRecorder) routingView {
		t.Helper()
		var v routingView
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(w.Body.String(), e)
		}
		return v
	}
	if w := request(s, "GET", url, "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}

	w := request(s, "GET", url, "", true)
	if v := view(w); w.Code != 200 || len(v.Tiers) != 0 || v.WhenExhausted != "block" || v.Limits["claude"] != 90 || len(v.Providers) != 2 {
		t.Fatal(w.Body.String())
	}

	body := `{"limits":{"claude":80,"codex":90},"when_exhausted":"block","tiers":{"fast":["claude:haiku","pi:openai-codex/gpt-6-luna"]}}`
	w = request(s, "PUT", url, body, true)
	v := view(w)
	if w.Code != 200 || len(v.Tiers) != 1 || v.Tiers[0].Name != "fast" || v.Tiers[0].Selected != "claude:haiku" || v.Limits["claude"] != 80 {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(v.Tiers[0].UsedBy) != 1 || v.Tiers[0].UsedBy[0] != "dev" || v.Tiers[0].Candidates[1].Ref != "pi:openai-codex/gpt-6-luna" {
		t.Fatalf("tier: %+v", v.Tiers[0])
	}

	// Quota moves the selection to the second candidate; the page only reads it back.
	snap = quota.Snapshot{Claude: quota.Provider{Five: routingWindowAt(93)}}
	v = view(request(s, "GET", url, "", true))
	if v.Tiers[0].Selected != "pi:openai-codex/gpt-6-luna" || !strings.Contains(v.Tiers[0].Reason, "Claude 5h 93%") || v.Tiers[0].Candidates[0].State != "exhausted" {
		t.Fatalf("tier: %+v", v.Tiers[0])
	}

	snap = quota.Snapshot{Claude: quota.Provider{Five: routingWindowAt(95)}, Codex: quota.Provider{Five: routingWindowAt(95)}}
	v = view(request(s, "GET", url, "", true))
	if v.Tiers[0].Selected != "" || !strings.Contains(v.Tiers[0].Reason, "exhausted") {
		t.Fatalf("tier: %+v", v.Tiers[0])
	}
	snap = quota.Snapshot{}

	before, _ := os.ReadFile(routing.Path())
	if w = request(s, "PUT", url, `{"tiers":{"my.tier":["claude:haiku"]}}`, true); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = request(s, "PUT", url, `{"limits":{"claude":80}}`, true); w.Code != 400 {
		t.Fatal("missing tiers accepted", w.Code)
	}
	if after, _ := os.ReadFile(routing.Path()); string(after) != string(before) {
		t.Fatal("rejected PUT changed the file")
	}

	os.WriteFile(routing.Path(), []byte("tiers: {my.tier: [claude:haiku]}\n"), 0600)
	if v = view(request(s, "GET", url, "", true)); v.Error == "" || len(v.Tiers) != 0 {
		t.Fatalf("invalid file: %+v", v)
	}
	if w = request(s, "PUT", url, body, true); w.Code != http.StatusConflict {
		t.Fatal("invalid file overwritten", w.Code)
	}
	if after, _ := os.ReadFile(routing.Path()); !strings.Contains(string(after), "my.tier") {
		t.Fatal("invalid file replaced")
	}
}
