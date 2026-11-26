package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/panes"
)

// extServer is a server with a config dir of its own and the given pane files (name -> HTML).
func extServer(t *testing.T, panes map[string]string) (*server, http.Handler) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(uiDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, html := range panes {
		if err := os.WriteFile(filepath.Join(uiDir(), name+".html"), []byte(html), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &server{token: "secret"}
	return s, s.handler()
}

func extReq(h http.Handler, method, path, body string, cookie bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie {
		r.AddCookie(&http.Cookie{Name: "tmux_agent", Value: "secret"})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestExtListAndServe(t *testing.T) {
	_, h := extServer(t, map[string]string{"hello": "<p>hi</p>", "a-b_c": "x"})
	os.Mkdir(filepath.Join(uiDir(), "dir.html"), 0o755) // a folder is not a pane
	os.WriteFile(filepath.Join(uiDir(), "-bad.html"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(uiDir(), "x.json"), []byte("x"), 0o644)

	if w := extReq(h, "GET", "/api/ext", "", true); w.Body.String() != `[{"name":"a-b_c","agent":false,"actions":false},{"name":"hello","agent":false,"actions":false}]`+"\n" {
		t.Fatalf("list: %d %q", w.Code, w.Body.String())
	}
	w := extReq(h, "GET", "/ext/hello.html", "", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<p>hi</p>") {
		t.Fatalf("pane: %d %q", w.Code, w.Body.String())
	}
	csp := w.Header().Get("Content-Security-Policy")
	if csp != paneCSP || strings.Contains(csp, "allow-same-origin") {
		t.Fatalf("pane CSP: %q", csp)
	}
	if w := extReq(h, "GET", "/", "", true); w.Header().Get("Content-Security-Policy") != "frame-src 'self'" {
		t.Fatalf("app shell CSP: %q", w.Header().Get("Content-Security-Policy"))
	}
}

func TestExtEmptyList(t *testing.T) {
	_, h := extServer(t, nil)
	if w := extReq(h, "GET", "/api/ext", "", true); w.Body.String() != "[]\n" {
		t.Fatalf("no ui folder: %q", w.Body.String())
	}
}

func TestExtAuth(t *testing.T) {
	_, h := extServer(t, map[string]string{"hello": "<p>hi</p>"})
	for _, p := range []string{"/ext/hello.html", "/api/ext", "/api/ext/hello/data"} {
		if w := extReq(h, "GET", p, "", false); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without cookie: %d", p, w.Code)
		}
	}
	if w := extReq(h, "GET", "/ext/sdk.js", "", false); w.Code != 200 || !strings.Contains(w.Body.String(), "tagent") {
		t.Fatalf("sdk without cookie: %d", w.Code)
	}
}

func TestExtBadNames(t *testing.T) {
	_, h := extServer(t, map[string]string{"hello": "<p>hi</p>"})
	for _, p := range []string{"/ext/..%2Fhello.html", "/ext/x.json", "/ext/bad%20name.html", "/ext/hello.htm", "/ext/nope.html", "/api/ext/..%2Fhello/data", "/api/ext/nope/data"} {
		if w := extReq(h, "GET", p, "", true); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
}

func TestExtData(t *testing.T) {
	_, h := extServer(t, map[string]string{"hello": "<p>hi</p>", "other": "x"})
	if w := extReq(h, "GET", "/api/ext/hello/data", "", true); w.Body.String() != "null" {
		t.Fatalf("absent: %q", w.Body.String())
	}
	if w := extReq(h, "PUT", "/api/ext/hello/data", `{"n":1}`, true); w.Code != 200 {
		t.Fatalf("put: %d %s", w.Code, w.Body.String())
	}
	if w := extReq(h, "GET", "/api/ext/hello/data", "", true); w.Body.String() != `{"n":1}` {
		t.Fatalf("round trip: %q", w.Body.String())
	}
	if w := extReq(h, "PUT", "/api/ext/hello/data", `{"n":`, true); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON accepted: %d", w.Code)
	}
	big := `"` + strings.Repeat("a", maxPaneData) + `"`
	if w := extReq(h, "PUT", "/api/ext/hello/data", big, true); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over 1 MiB accepted: %d", w.Code)
	}
	if w := extReq(h, "GET", "/api/ext/hello/data", "", true); w.Body.String() != `{"n":1}` {
		t.Fatalf("rejected write changed the file: %q", w.Body.String())
	}
	if b, _ := os.ReadFile(filepath.Join(uiDir(), "hello.json")); string(b) != `{"n":1}` {
		t.Fatalf("file: %q", b)
	}
}

func TestExtCreateDeleteFromUI(t *testing.T) {
	_, h := extServer(t, map[string]string{"mine": "<p>x</p>"})
	if w := extReq(h, "POST", "/api/ext", `{"name":"new-one","html":"<p>n</p>"}`, true); w.Code != 200 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := extReq(h, "GET", "/ext/new-one.html", "", true); !strings.Contains(w.Body.String(), "<p>n</p>") {
		t.Fatalf("created pane: %q", w.Body.String())
	}
	for _, body := range []string{`{"name":"mine","html":"<p>y</p>"}`, `{"name":"../x","html":"<p>y</p>"}`, `{"name":"a.b","html":"y"}`, `{"name":"e","html":" "}`, `{"name":"big","html":"` + strings.Repeat("a", 300<<10) + `"}`, `nope`} {
		if w := extReq(h, "POST", "/api/ext", body, true); w.Code != http.StatusBadRequest {
			t.Fatalf("%.40s: %d", body, w.Code)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(uiDir(), "mine.html")); string(b) != "<p>x</p>" {
		t.Fatalf("existing pane changed: %q", b)
	}
	extReq(h, "PUT", "/api/ext/new-one/data", `{"n":1}`, true)
	if w := extReq(h, "DELETE", "/api/ext/new-one", "", true); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(uiDir(), "new-one.json")); err == nil {
		t.Fatal("data left behind")
	}
	if w := extReq(h, "DELETE", "/api/ext/new-one", "", true); w.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d", w.Code)
	}
	if w := extReq(h, "DELETE", "/api/ext/..%2Fmine", "", true); w.Code != http.StatusNotFound {
		t.Fatalf("traversal: %d", w.Code)
	}
	if w := extReq(h, "POST", "/api/ext", `{"name":"x","html":"y"}`, false); w.Code != http.StatusUnauthorized {
		t.Fatalf("create without cookie: %d", w.Code)
	}
}

func TestExtGrantBoundToContent(t *testing.T) {
	_, h := extServer(t, map[string]string{"mine": "<p>x</p>"})
	if err := panes.Put("bot", "<p>v1</p>", "dev-1"); err != nil {
		t.Fatal(err)
	}
	list := func() string { return extReq(h, "GET", "/api/ext", "", true).Body.String() }
	if !strings.Contains(list(), `{"name":"bot","agent":true,"actions":false}`) {
		t.Fatalf("new agent pane: %s", list())
	}
	if w := extReq(h, "PUT", "/api/ext/bot/grant", `{"allow":true}`, true); w.Code != 200 {
		t.Fatalf("grant: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(list(), `{"name":"bot","agent":true,"actions":true}`) {
		t.Fatalf("granted: %s", list())
	}
	// The agent changes its pane: the grant is gone. Writing the file by any other way voids it too.
	if err := panes.Put("bot", "<p>v2</p>", "dev-1"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list(), `{"name":"bot","agent":true,"actions":false}`) {
		t.Fatalf("revoked by rewrite: %s", list())
	}
	extReq(h, "PUT", "/api/ext/bot/grant", `{"allow":true}`, true)
	os.WriteFile(filepath.Join(uiDir(), "bot.html"), []byte("<p>v3</p>"), 0o644)
	if !strings.Contains(list(), `{"name":"bot","agent":true,"actions":false}`) {
		t.Fatalf("revoked by edit: %s", list())
	}
	extReq(h, "PUT", "/api/ext/bot/grant", `{"allow":true}`, true)
	extReq(h, "PUT", "/api/ext/bot/grant", `{"allow":false}`, true)
	if !strings.Contains(list(), `{"name":"bot","agent":true,"actions":false}`) {
		t.Fatalf("revoke: %s", list())
	}
	if w := extReq(h, "PUT", "/api/ext/mine/grant", `{"allow":true}`, true); w.Code != http.StatusBadRequest {
		t.Fatalf("grant on a user pane: %d", w.Code)
	}
	if w := extReq(h, "PUT", "/api/ext/nope/grant", `{"allow":true}`, true); w.Code != http.StatusBadRequest {
		t.Fatalf("grant on no pane: %d", w.Code)
	}
	if w := extReq(h, "PUT", "/api/ext/bot/grant", `{"allow":true}`, false); w.Code != http.StatusUnauthorized {
		t.Fatalf("grant without cookie: %d", w.Code)
	}
}

func TestExtDamagedMetaIsAgentReadOnly(t *testing.T) {
	_, h := extServer(t, map[string]string{"mine": "<p>x</p>"})
	os.WriteFile(filepath.Join(uiDir(), "mine.meta.json"), []byte("{"), 0o644)
	if w := extReq(h, "GET", "/api/ext", "", true); !strings.Contains(w.Body.String(), `"agent":true,"actions":false`) {
		t.Fatalf("damaged sidecar: %s", w.Body.String())
	}
}
