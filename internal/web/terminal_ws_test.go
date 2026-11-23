package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecodeOutput(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"hello", "hello"}, {`\033[31m\134\000\377`, "\x1b[31m\\\x00\xff"}, {"é", "é"}} {
		b, err := decodeOutput(tc.in)
		if err != nil || string(b) != tc.want {
			t.Fatalf("%q: %q %v", tc.in, b, err)
		}
	}
	for _, s := range []string{`\`, `\12`, `\888`, `\400`} {
		if _, err := decodeOutput(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}

func TestTerminalWSAuthOrigin(t *testing.T) {
	s := &server{token: "secret"}
	for _, tc := range []struct {
		cookie, origin string
		want           int
	}{
		{"", "http://example.com", 401},
		{"wrong", "http://example.com", 401},
		{"secret", "http://evil.com", 403},
		{"secret", "https://example.com", 403},
		{"secret", "", 403},
		// Same-origin requests pass security checks, then fail pane validation.
		{"secret", "http://example.com", 400},
	} {
		r := httptest.NewRequest("GET", "http://example.com/api/pane/ws?pane=invalid", nil)
		r.Header.Set("Origin", tc.origin)
		if tc.cookie != "" {
			r.AddCookie(&http.Cookie{Name: "tmux_agent", Value: tc.cookie})
		}
		w := httptest.NewRecorder()
		s.handler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%+v: got %d", tc, w.Code)
		}
	}
}
