package web

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"tagent/internal/apppath"
)

const maxUpload = 10 << 20

// Images only: what the chat attaches so an agent can open the file by its path.
var uploadTypes = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp"}
var uploadName = regexp.MustCompile(`^[0-9a-f]{24}\.(png|jpg|gif|webp)$`)

func uploadsDir() string {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, _ := os.UserHomeDir()
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(apppath.Dir(state), "uploads")
}

// upload stores one image (multipart field "file") and returns its absolute path, which an agent on this
// machine can read, and a URL the web page can show. The file name is random; the client's is never used.
func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	f, _, err := r.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "image exceeds 10 MiB", 413)
		} else {
			http.Error(w, "expected a multipart field named file", 400)
		}
		return
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	ext, ok := uploadTypes[http.DetectContentType(head[:n])]
	if !ok {
		http.Error(w, "only PNG, JPEG, GIF and WebP images are accepted", 415)
		return
	}
	dir := uploadsDir()
	if err = os.MkdirAll(dir, 0o700); err != nil {
		fail(w, err)
		return
	}
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	name := hex.EncodeToString(id) + ext
	path := filepath.Join(dir, name)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		fail(w, err)
		return
	}
	size, err := io.Copy(out, io.MultiReader(bytes.NewReader(head[:n]), io.LimitReader(f, maxUpload)))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil || size > maxUpload {
		_ = os.Remove(path)
		http.Error(w, "upload failed or exceeds 10 MiB", 413)
		return
	}
	writeJSON(w, map[string]any{"path": path, "url": "/api/uploads/" + name, "name": name, "size": size})
}

// serveUpload shows an uploaded image. Only names this server generated are served, so no path can escape the folder.
func (s *server) serveUpload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !uploadName.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, filepath.Join(uploadsDir(), name))
}
