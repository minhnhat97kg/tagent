package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"tagent/internal/metrics"
	"tagent/internal/quota"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/usage"
	"testing"
)

func request(s *server, method, path, body string, auth bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if auth {
		r.AddCookie(&http.Cookie{Name: "tmux_agent", Value: s.token})
	}
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	return w
}
func TestOverviewAPI(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	s := &server{token: "secret", dir: t.TempDir(), deps: apiDeps{sessions: func() []tmux.SessionInfo { return nil }, refresh: func(string, []tmux.SessionInfo) metrics.State { return metrics.State{} }, quota: func() quota.Snapshot { return quota.Snapshot{} }}}
	if w := request(s, "GET", "/api/overview", "", false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	w := request(s, "GET", "/api/overview", "", true)
	var data struct {
		Dir       string
		Providers []struct {
			Used *float64 `json:"usedPercent"`
		}
		Tokens    metrics.Window
		Attention metrics.Attention
		Now       []string
	}
	if e := json.Unmarshal(w.Body.Bytes(), &data); e != nil {
		t.Fatal(w.Body.String(), e)
	}
	if w.Code != 200 || data.Dir != s.dir || len(data.Providers) != 2 || data.Providers[0].Used != nil || len(data.Tokens.Days) != 7 || data.Now == nil || data.Attention.Items == nil {
		t.Fatal(w.Body.String())
	}
}

func TestTicketUsageAPI(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	tk, _ := tasks.Add(scope, "x", nil)
	_ = tasks.RecordDispatch(scope, tk.ID, tasks.Event{Role: "dev", Harness: "pi", PaneName: "dev-1", Pane: "%1"})
	state := metrics.State{Sessions: map[string]metrics.Session{"pi:a": {Key: "pi:a", Harness: "pi", Agent: "dev-1", HasUsage: true, Usage: usage.Count{Total: 99}}}}
	s := &server{token: "secret", dir: dir, deps: apiDeps{sessions: func() []tmux.SessionInfo { return nil }, refresh: func(string, []tmux.SessionInfo) metrics.State { return state }}}
	w := request(s, "GET", "/api/tasks/"+tk.ID+"/usage", "", true)
	var u metrics.TicketUsage
	if e := json.Unmarshal(w.Body.Bytes(), &u); e != nil || w.Code != 200 || u.Total.Total != 99 || len(u.Jobs) != 1 {
		t.Fatal(w.Code, w.Body.String(), e)
	}
	if w = request(s, "GET", "/api/tasks/nope/usage", "", true); w.Code != 404 {
		t.Fatal(w.Code)
	}
}
