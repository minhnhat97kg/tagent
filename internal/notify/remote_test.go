package notify

import (
	"io"
	"net/http"
	"net/http/httptest"
	"tagent/internal/tmux"
	"testing"
)

func TestPostRemotePlainText(t *testing.T) {
	var gotTitle, gotBody, gotPath, gotTags string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotTitle, gotBody, gotPath = r.Header.Get("Title"), string(b), r.URL.Path
		gotTags = r.Header.Get("Tags")
	}))
	defer srv.Close()

	if err := postRemote(srv.URL+"/topic", "api", "Task completed"); err != nil {
		t.Fatal(err)
	}
	if gotTitle != "api" || gotBody != "Task completed" || gotPath != "/topic" || gotTags != "robot" {
		t.Errorf("ntfy: title %q body %q", gotTitle, gotBody)
	}
}

func TestDisabledAndFailedDelivery(t *testing.T) {
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	Send("title", "body")
	fake.Set("", "show", "-gv", notifyURLOption)
	Send("title", "body")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	if postRemote(srv.URL, "title", "body") == nil {
		t.Fatal("expected non-success error")
	}
	fake.Set(srv.URL, "show", "-gv", notifyURLOption)
	Send("title", "body")
}
