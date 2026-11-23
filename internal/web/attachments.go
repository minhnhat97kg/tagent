package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"tagent/internal/tasks"
	"unicode/utf8"
)

func (s *server) attachments(w http.ResponseWriter, r *http.Request) {
	dir := s.projectDir(r)
	scope := tasks.Scope(dir)
	id := r.PathValue("id")
	if _, err := tasks.Get(scope, id); err != nil {
		http.Error(w, "ticket not found", 404)
		return
	}
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 40<<20)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			if r.MultipartForm != nil {
				defer r.MultipartForm.RemoveAll()
			}
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(w, "request exceeds 40 MiB", 413)
			} else {
				http.Error(w, "invalid multipart upload", 400)
			}
			return
		}
		defer r.MultipartForm.RemoveAll()
		files := r.MultipartForm.File["files"]
		if len(files) > 4 {
			http.Error(w, "maximum four files", 413)
			return
		}
		tmp, err := os.MkdirTemp("", "tagent-upload-")
		if err != nil {
			fail(w, err)
			return
		}
		defer os.RemoveAll(tmp)
		paths := []string{}
		for i, h := range files {
			if h.Size > 10<<20 {
				http.Error(w, "file exceeds 10 MiB", 413)
				return
			}
			sub := filepath.Join(tmp, fmt.Sprint(i))
			if err = os.Mkdir(sub, 0700); err != nil {
				fail(w, err)
				return
			}
			name := filepath.Base(strings.ReplaceAll(h.Filename, "\\", "/"))
			if name == "." || name == ".." || name == "" {
				http.Error(w, "invalid filename", 400)
				return
			}
			p := filepath.Join(sub, name)
			in, e := h.Open()
			if e != nil {
				fail(w, e)
				return
			}
			out, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				in.Close()
				fail(w, e)
				return
			}
			n, e := io.Copy(out, io.LimitReader(in, (10<<20)+1))
			in.Close()
			ce := out.Close()
			if e != nil || ce != nil || n > 10<<20 {
				http.Error(w, "upload failed or exceeds 10 MiB", 413)
				return
			}
			paths = append(paths, p)
		}
		err = tasks.Comment(scope, id, "user", r.FormValue("text"), paths, tasks.MutationOptions{Actor: "user"})
		if _, ok := err.(*tasks.CommentNotice); ok {
			writeJSON(w, map[string]string{"notice": err.Error()})
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.WriteHeader(http.StatusCreated)
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		list, err := tasks.AttachmentList(scope, dir, id)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
		return
	}
	f, a, err := tasks.OpenTicketAttachment(scope, dir, id, ref)
	if err != nil {
		http.Error(w, "attachment unavailable", 404)
		return
	}
	defer f.Close()
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Name}))
		io.Copy(w, f)
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	if err != nil {
		fail(w, err)
		return
	}
	truncated := len(data) > 256<<10
	if truncated {
		data = data[:256<<10]
	}
	if r.URL.Query().Get("full") == "1" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		f.Seek(0, 0)
		io.Copy(w, f)
		return
	}
	kind := http.DetectContentType(data)
	if kind == "image/png" || kind == "image/jpeg" || kind == "image/gif" || kind == "image/webp" {
		w.Header().Set("Content-Type", kind)
		f.Seek(0, 0)
		io.Copy(w, f)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if truncated {
		for n := 0; n < 3 && !utf8.Valid(data) && len(data) > 0; n++ {
			data = data[:len(data)-1]
		}
	}
	binary := !utf8.Valid(data) || strings.ContainsRune(string(data), 0)
	text := string(data)
	if binary {
		text = "Binary file — use Download."
		truncated = false
	}
	json.NewEncoder(w).Encode(map[string]any{"text": text, "binary": binary, "truncated": truncated, "size": a.Size})
}
