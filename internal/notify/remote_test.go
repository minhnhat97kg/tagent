package notify

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"tmux-agent/internal/tmux"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPostRemoteNtfyAndTelegram(t *testing.T) {
	var gotTitle, gotBody, gotPath, gotChat string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotTitle, gotBody, gotPath = r.Header.Get("Title"), string(b), r.URL.Path
		gotChat = r.URL.Query().Get("chat_id")
	}))
	defer srv.Close()

	if err := postRemote(srv.URL+"/topic", "api", "Task completed"); err != nil {
		t.Fatal(err)
	}
	if gotTitle != "api" || gotBody != "Task completed" {
		t.Errorf("ntfy: title %q body %q", gotTitle, gotBody)
	}
	old := remoteClient
	defer func() { remoteClient = old }()
	u, _ := url.Parse(srv.URL)
	remoteClient = &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme, r.URL.Host = u.Scheme, u.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	if err := postRemote("https://api.telegram.org/bot123/sendMessage?chat_id=9", "api", "done"); err != nil {
		t.Fatal(err)
	}
	if gotChat != "9" || gotBody != "text=api%0Adone" || gotPath != "/bot123/sendMessage" {
		t.Errorf("telegram: path %q body %q", gotPath, gotBody)
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
