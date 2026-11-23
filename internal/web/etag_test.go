package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEtaggedRevalidates(t *testing.T) {
	h := etagged(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"a":1}`) }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/project", nil))
	tag := w.Header().Get("ETag")
	if w.Code != 200 || tag == "" || w.Body.String() != `{"a":1}` {
		t.Fatalf("first: %d %q %q", w.Code, tag, w.Body.String())
	}
	r := httptest.NewRequest("GET", "/api/project", nil)
	r.Header.Set("If-None-Match", tag)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 304 || w.Body.Len() != 0 {
		t.Fatalf("revalidate: %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/tasks", nil))
	if w.Header().Get("ETag") != "" {
		t.Fatal("POST must not be etagged")
	}
}
