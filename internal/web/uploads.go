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
	"strings"

	"tagent/internal/apppath"
)

const maxUpload = 10 << 20

// Images are shown inline; every other file is only ever offered as a download.
var imageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}
var uploadID = regexp.MustCompile(`^[0-9a-f]{24}$`)
var uploadFile = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func uploadsDir() string {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, _ := os.UserHomeDir()
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(apppath.Dir(state), "uploads")
}

// safeName keeps the client's file name readable for the agent but never lets it steer the path.
func safeName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Trim(unsafeName.ReplaceAllString(name, "_"), "._-")
	if len(name) > 100 {
		ext := filepath.Ext(name)
		if len(ext) > 12 {
			ext = ""
		}
		name = name[:100-len(ext)] + ext
	}
	if name == "" {
		return "file"
	}
	return name
}

// upload stores one file (multipart field "file") as uploads/<random id>/<clean name> and returns its
// absolute path, which an agent on this machine can read, and a URL for the web page.
func (s *server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+(1<<20))
	f, h, err := r.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "file exceeds 10 MiB", 413)
		} else {
			http.Error(w, "expected a multipart field named file", 400)
		}
		return
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	image := imageTypes[http.DetectContentType(head[:n])]
	idb := make([]byte, 12)
	_, _ = rand.Read(idb)
	id, name := hex.EncodeToString(idb), safeName(h.Filename)
	dir := filepath.Join(uploadsDir(), id)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		fail(w, err)
		return
	}
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
		_ = os.RemoveAll(dir)
		http.Error(w, "upload failed or exceeds 10 MiB", 413)
		return
	}
	writeJSON(w, map[string]any{"path": path, "url": "/api/uploads/" + id + "/" + name, "name": name, "size": size, "image": image})
}

// serveUpload returns an uploaded file. Only names this server generated are served, so no path can escape
// the folder; a file that is not an image is a download, never rendered by the browser.
func (s *server) serveUpload(w http.ResponseWriter, r *http.Request) {
	id, name := r.PathValue("id"), r.PathValue("name")
	if !uploadID.MatchString(id) || !uploadFile.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(uploadsDir(), id, name)
	in, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer in.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(in, head)
	kind := http.DetectContentType(head[:n])
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox")
	h.Set("Cache-Control", "private, max-age=86400")
	if imageTypes[kind] {
		h.Set("Content-Type", kind)
	} else {
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	}
	st, err := in.Stat()
	if err != nil {
		fail(w, err)
		return
	}
	http.ServeContent(w, r, name, st.ModTime(), in)
}
