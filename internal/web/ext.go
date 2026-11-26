package web

// Custom panes: user HTML files in <config dir>/ui, each shown in a sandboxed iframe that talks to /api/*
// through the app's postMessage bridge (web/src/lib/extBridge.ts, sdk in ext_sdk.js). A pane is trusted code.

import (
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"tagent/internal/settings"
)

//go:embed ext_sdk.js
var extSDK []byte

const (
	paneCSP     = "sandbox allow-scripts; frame-ancestors 'self'" // no allow-same-origin: the pane never gets the app origin
	maxPaneData = 1 << 20
)

var paneName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// uiDir is the folder of pane files, next to config.conf. A relative path (a relative XDG_CONFIG_HOME would
// resolve against the working directory, maybe a repo) means no panes.
func uiDir() string {
	d := filepath.Dir(settings.Path())
	if !filepath.IsAbs(d) {
		return ""
	}
	return filepath.Join(d, "ui")
}

// paneFile is the HTML file of a valid pane name, or "" when there is no such pane.
func paneFile(name string) string {
	dir := uiDir()
	if dir == "" || !paneName.MatchString(name) {
		return ""
	}
	f := filepath.Join(dir, name+".html")
	if fi, err := os.Stat(f); err != nil || fi.IsDir() {
		return ""
	}
	return f
}

// extList is GET /api/ext: the names of the panes, sorted.
func (s *server) extList(w http.ResponseWriter, r *http.Request) {
	names := []string{}
	entries, _ := os.ReadDir(uiDir()) // a missing folder lists nothing
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".html"); ok && paneFile(name) != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	writeJSON(w, names)
}

// extPane is GET /ext/{file}: only <name>.html. The CSP keeps the pane sandboxed even when its URL is opened on its own.
func (s *server) extPane(w http.ResponseWriter, r *http.Request) {
	name, ok := strings.CutSuffix(r.PathValue("file"), ".html")
	f := paneFile(name)
	if !ok || f == "" {
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(f)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", paneCSP)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-cache")
	w.Write(b)
}

// extSDKFile is GET /ext/sdk.js. It needs no auth: a sandboxed frame's subresource request carries no cookie.
func (s *server) extSDKFile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Write(extSDK)
}

// extData is GET or PUT /api/ext/{name}/data: the JSON state of a pane, kept in <name>.json next to its HTML.
func (s *server) extData(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	html := paneFile(name)
	if html == "" {
		http.NotFound(w, r)
		return
	}
	file := strings.TrimSuffix(html, ".html") + ".json"
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodGet {
		b, err := os.ReadFile(file)
		if errors.Is(err, os.ErrNotExist) {
			b = []byte("null")
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(b)
		return
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, maxPaneData+1))
	if err != nil {
		fail(w, err)
		return
	}
	if len(b) > maxPaneData {
		http.Error(w, "data over 1 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	if !json.Valid(b) {
		fail(w, errors.New("body must be one JSON value"))
		return
	}
	// Write a temp file and rename it, so a reload never reads half a file.
	tmp, err := os.CreateTemp(filepath.Dir(file), name+".json.*")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), file)
	}
	if err != nil {
		os.Remove(tmp.Name())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("{}"))
}
