package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	if w := extReq(h, "GET", "/api/ext", "", true); w.Body.String() != `["a-b_c","hello"]`+"\n" {
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
