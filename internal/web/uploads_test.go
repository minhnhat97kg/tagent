package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x9a\xa0\xa0\x00\x00\x00\x00IEND\xaeB`\x82")

func post(t *testing.T, s *server, name string, data []byte) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	mw.Close()
	r := httptest.NewRequest("POST", "/api/uploads", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.upload(w, r)
	return w
}

func serve(s *server, id, name string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/api/uploads/x/y", nil)
	r.SetPathValue("id", id)
	r.SetPathValue("name", name)
	rec := httptest.NewRecorder()
	s.serveUpload(rec, r)
	return rec
}

type uploaded struct {
	Path, URL, Name string
	Image           bool
}

func TestUploadImageAndFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s := &server{}
	w := post(t, s, "../../evil shot.png", tinyPNG)
	if w.Code != 200 {
		t.Fatalf("png: %d %s", w.Code, w.Body)
	}
	var img uploaded
	json.Unmarshal(w.Body.Bytes(), &img)
	if !img.Image || img.Name != "evil_shot.png" || !strings.Contains(img.Path, "/uploads/") || strings.Contains(img.Path, "..") {
		t.Fatalf("name must be cleaned and kept: %+v", img)
	}
	if b, err := os.ReadFile(img.Path); err != nil || !bytes.Equal(b, tinyPNG) {
		t.Fatalf("stored file differs: %v", err)
	}
	id := strings.Split(strings.TrimPrefix(img.URL, "/api/uploads/"), "/")[0]
	if rec := serve(s, id, img.Name); rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/png") || rec.Header().Get("Content-Disposition") != "" {
		t.Fatalf("image should be inline: %d %v", rec.Code, rec.Header())
	}

	// any other file is stored under its own name but only ever served as a download
	w = post(t, s, "report.html", []byte("<html><script>alert(1)</script></html>"))
	var doc uploaded
	json.Unmarshal(w.Body.Bytes(), &doc)
	if w.Code != 200 || doc.Image || doc.Name != "report.html" {
		t.Fatalf("file: %d %+v", w.Code, doc)
	}
	id = strings.Split(strings.TrimPrefix(doc.URL, "/api/uploads/"), "/")[0]
	rec := serve(s, id, doc.Name)
	if rec.Header().Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Content-Security-Policy") != "sandbox" {
		t.Fatalf("html must download, not render: %v", rec.Header())
	}
}

func TestUploadRejectsAndConfines(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s := &server{}
	if w := post(t, s, "big.bin", make([]byte, maxUpload+10)); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize accepted: %d", w.Code)
	}
	for _, c := range [][2]string{{"..", "x"}, {"0123456789abcdef01234567", "../../etc/passwd"}, {"0123456789abcdef01234567", ".hidden"}, {"../etc", "passwd"}, {"0123456789abcdef01234567", "a/b"}} {
		if rec := serve(s, c[0], c[1]); rec.Code != 404 {
			t.Errorf("%q served: %d", c, rec.Code)
		}
	}
	if safeName("../../..") != "file" || safeName("  .env ") != "env" || len(safeName(strings.Repeat("a", 300)+".pdf")) > 100 {
		t.Errorf("safeName: %q %q", safeName("../../.."), safeName("  .env "))
	}
}
