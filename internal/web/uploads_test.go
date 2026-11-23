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

func TestUploadImage(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s := &server{}
	w := post(t, s, "../../evil.png", tinyPNG)
	if w.Code != 200 {
		t.Fatalf("png: %d %s", w.Code, w.Body)
	}
	var got struct{ Path, URL, Name string }
	json.Unmarshal(w.Body.Bytes(), &got)
	if !uploadName.MatchString(got.Name) || !strings.HasSuffix(got.Path, "/uploads/"+got.Name) || strings.Contains(got.Path, "evil") {
		t.Fatalf("name must be generated, not the client's: %+v", got)
	}
	if b, err := os.ReadFile(got.Path); err != nil || !bytes.Equal(b, tinyPNG) {
		t.Fatalf("stored file differs: %v", err)
	}
	// it is served back, with the right type
	r := httptest.NewRequest("GET", got.URL, nil)
	r.SetPathValue("name", got.Name)
	rec := httptest.NewRecorder()
	s.serveUpload(rec, r)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/png") {
		t.Fatalf("serve: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestUploadRejects(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s := &server{}
	if w := post(t, s, "a.png", []byte("<html><script>alert(1)</script></html>")); w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-image accepted: %d", w.Code)
	}
	if w := post(t, s, "big.png", append(append([]byte{}, tinyPNG...), make([]byte, maxUpload+10)...)); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize accepted: %d", w.Code)
	}
	for _, name := range []string{"../../etc/passwd", "x.png", "0123456789abcdef01234567.png/../../x", "0123456789abcdef01234567.html"} {
		r := httptest.NewRequest("GET", "/api/uploads/x", nil)
		r.SetPathValue("name", name)
		rec := httptest.NewRecorder()
		s.serveUpload(rec, r)
		if rec.Code != 404 {
			t.Errorf("%q served: %d", name, rec.Code)
		}
	}
}
