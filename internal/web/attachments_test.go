package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"tagent/internal/tasks"
	"testing"
)

func TestAttachmentUploadAndViewer(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "Attachments", nil)
	if err != nil {
		t.Fatal(err)
	}
	s := server{dir: dir}
	request := func(method, query string, body io.Reader, ct string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/tasks/"+ticket.ID+"/attachments"+query, body)
		r.SetPathValue("id", ticket.ID)
		r.Header.Set("Content-Type", ct)
		w := httptest.NewRecorder()
		s.attachments(w, r)
		return w
	}
	upload := func(name, content string) *httptest.ResponseRecorder {
		var b bytes.Buffer
		m := multipart.NewWriter(&b)
		f, _ := m.CreateFormFile("files", name)
		io.WriteString(f, content)
		m.Close()
		return request("POST", "", &b, m.FormDataContentType())
	}
	if w := upload("hello.md", "# Hello\n"); w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	var list []tasks.TicketAttachment
	w := request("GET", "", nil, "")
	if err = json.Unmarshal(w.Body.Bytes(), &list); err != nil || len(list) != 1 {
		t.Fatalf("list: %s %v", w.Body.String(), err)
	}
	q := "?ref=" + url.QueryEscape(list[0].Ref)
	w = request("GET", q, nil, "")
	if !strings.Contains(w.Body.String(), "Hello") || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal(w)
	}
	w = request("GET", q+"&download=1", nil, "")
	if w.Body.String() != "# Hello\n" || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal(w)
	}
	if w = request("GET", "?ref=../escape", nil, ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w = upload("huge.log", strings.Repeat("x", (10<<20)+1)); w.Code != 413 {
		t.Fatal(w.Code)
	}
	got, _ := tasks.Get(scope, ticket.ID)
	if len(got.Comments) != 1 {
		t.Fatal("partial oversize comment")
	}
	if w = upload("large.log", strings.Repeat("x", 300<<10)); w.Code != 201 {
		t.Fatal(w.Code)
	}
	list, _ = tasks.AttachmentList(scope, dir, ticket.ID)
	for _, a := range list {
		if a.Name == "large.log" {
			w = request("GET", "?ref="+a.Ref, nil, "")
			if !strings.Contains(w.Body.String(), `"truncated":true`) {
				t.Fatal("unbounded preview")
			}
		}
	}
	if w = upload("active.html", "<script>alert(1)</script>"); w.Code != 201 {
		t.Fatal(w.Code)
	}
	list, _ = tasks.AttachmentList(scope, dir, ticket.ID)
	for _, a := range list {
		if a.Name == "active.html" {
			w = request("GET", "?ref="+a.Ref, nil, "")
			if w.Header().Get("Content-Type") != "application/json" {
				t.Fatal("active response")
			}
		}
	}
	source := filepath.Join(dir, "link")
	os.Symlink("/etc/passwd", source)
	if err = tasks.Comment(scope, ticket.ID, "user", "", []string{source}); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestFilesOnlyUploadCreatesUserComment(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "Files only", nil)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	f, _ := m.CreateFormFile("files", "ci.log")
	io.WriteString(f, "ok\n")
	m.Close()
	r := httptest.NewRequest("POST", "/api/tasks/"+ticket.ID+"/attachments", &b)
	r.SetPathValue("id", ticket.ID)
	r.Header.Set("Content-Type", m.FormDataContentType())
	w := httptest.NewRecorder()
	(&server{dir: dir}).attachments(w, r)
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	got, _ := tasks.Get(scope, ticket.ID)
	if len(got.Comments) != 1 || got.Comments[0].Author != "user" || len(got.Comments[0].Attachments) != 1 {
		t.Fatalf("%+v", got.Comments)
	}
}
