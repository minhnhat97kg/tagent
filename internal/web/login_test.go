package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginForm(t *testing.T) {
	s := &server{token: "secret", dir: t.TempDir()}
	login := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/login", strings.NewReader("token="+token))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.handler().ServeHTTP(w, r)
		return w
	}
	if w := request(s, "GET", "/", "", false); w.Code != 401 || !strings.Contains(w.Body.String(), `action="/login"`) {
		t.Fatalf("no cookie must show the form: %d %s", w.Code, w.Body)
	}
	if w := login("nope"); w.Code != 401 || w.Header().Get("Set-Cookie") != "" || !strings.Contains(w.Body.String(), "Wrong token") {
		t.Fatalf("wrong token: %d %q %s", w.Code, w.Header().Get("Set-Cookie"), w.Body)
	}
	w := login("+secret+") // "+" decodes to a space: pasted whitespace is trimmed
	if c := w.Header().Get("Set-Cookie"); w.Code != 302 || w.Header().Get("Location") != "/" || !strings.Contains(c, "tmux_agent=secret") || !strings.Contains(c, "HttpOnly") {
		t.Fatalf("right token: %d %q", w.Code, c)
	}
}
