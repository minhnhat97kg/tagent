package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"tagent/internal/tmux"
)

func TestWorkspacesAPI(t *testing.T) {
	defer tmux.SetRunner(tmux.NewFake())()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := server{token: "tok", dir: t.TempDir()}
	h := s.handler()
	do := func(method, target, body string, auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if auth {
			req.AddCookie(&http.Cookie{Name: "tmux_agent", Value: "tok"})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	projects := func() []string {
		var st struct {
			Projects []string `json:"projects"`
		}
		json.Unmarshal(do("GET", "/api/state", "", true).Body.Bytes(), &st)
		return st.Projects
	}
	if w := do("GET", "/api/state", "", false); w.Code != http.StatusUnauthorized {
		t.Fatalf("no cookie: %d", w.Code)
	}

	repo := t.TempDir()
	os.Mkdir(filepath.Join(repo, "sub"), 0700)
	os.Mkdir(filepath.Join(repo, ".hidden"), 0700)
	if w := do("POST", "/api/workspaces", `{"Dir":"`+repo+`"}`, true); w.Code != 200 || !strings.Contains(w.Body.String(), repo) {
		t.Fatalf("add: %d %s", w.Code, w.Body.String())
	}
	if w := do("POST", "/api/workspaces", `{"Dir":"relative"}`, true); w.Code != 400 {
		t.Fatalf("relative add: %d", w.Code)
	}
	if p := projects(); !slices.Contains(p, repo) {
		t.Fatalf("added workspace missing from state: %v", p)
	}

	var d struct {
		Path, Parent string
		Dirs         []string
	}
	json.Unmarshal(do("GET", "/api/dirs?path="+repo, "", true).Body.Bytes(), &d)
	if d.Path != repo || d.Parent != filepath.Dir(repo) || !slices.Equal(d.Dirs, []string{"sub"}) {
		t.Fatalf("dirs: %+v", d)
	}
	if w := do("GET", "/api/dirs?path=rel", "", true); w.Code != 400 {
		t.Fatalf("relative dirs: %d", w.Code)
	}
	if w := do("GET", "/api/dirs?path=/", "", true); w.Code != 200 || !strings.Contains(w.Body.String(), `"parent":""`) {
		t.Fatalf("root parent: %d %s", w.Code, w.Body.String())
	}

	if w := do("DELETE", "/api/workspaces?dir="+repo, "", true); w.Code != 200 {
		t.Fatalf("remove: %d", w.Code)
	}
	if p := projects(); slices.Contains(p, repo) {
		t.Fatalf("removed workspace still in state: %v", p)
	}
}
