package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"strings"
)

// etagged lets polled GET /api responses revalidate: an unchanged body costs a 304 with no payload
// (the board JSON is ~100KB every 2s per viewer otherwise, which hurts over Tailscale and on phones).
func etagged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/pane/ws" {
			next.ServeHTTP(w, r)
			return
		}
		rec := &bufferedResponse{header: w.Header(), code: http.StatusOK}
		next.ServeHTTP(rec, r)
		if rec.code == http.StatusOK {
			sum := sha1.Sum(rec.body.Bytes())
			tag := `"` + hex.EncodeToString(sum[:8]) + `"`
			w.Header().Set("ETag", tag)
			w.Header().Set("Cache-Control", "no-cache")
			if r.Header.Get("If-None-Match") == tag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		if rec.body.Len() > 1024 && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Add("Vary", "Accept-Encoding")
			w.WriteHeader(rec.code)
			gz := gzip.NewWriter(w)
			gz.Write(rec.body.Bytes())
			gz.Close()
			return
		}
		w.WriteHeader(rec.code)
		w.Write(rec.body.Bytes())
	})
}

// gzipped compresses static files (the JS bundle is ~830KB raw, ~240KB gzipped) when the browser accepts it.
func gzipped(w http.ResponseWriter, r *http.Request, serve func(http.ResponseWriter, *http.Request)) {
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || r.Header.Get("Range") != "" {
		serve(w, r)
		return
	}
	gz := gzip.NewWriter(w)
	defer gz.Close()
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Add("Vary", "Accept-Encoding")
	serve(&gzipResponse{ResponseWriter: w, gz: gz}, r)
}

type gzipResponse struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (g *gzipResponse) WriteHeader(code int) {
	g.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}
func (g *gzipResponse) Write(p []byte) (int, error) {
	g.Header().Del("Content-Length")
	return g.gz.Write(p)
}

type bufferedResponse struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header         { return b.header }
func (b *bufferedResponse) WriteHeader(code int)        { b.code = code }
func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }
