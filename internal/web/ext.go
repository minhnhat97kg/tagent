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
	"strings"

	"tagent/internal/panes"
)

//go:embed ext_sdk.js
var extSDK []byte

const (
	paneCSP     = "sandbox allow-scripts; frame-ancestors 'self'" // no allow-same-origin: the pane never gets the app origin
	maxPaneData = 1 << 20
)

// uiDir and paneFile are the pane store's folder and a pane's HTML file (internal/panes).
func uiDir() string               { return panes.Dir() }
func paneFile(name string) string { return panes.File(name) }

// extList is GET /api/ext: the panes, sorted, each with whether an agent made it and may act (see internal/panes).
func (s *server) extList(w http.ResponseWriter, r *http.Request) { writeJSON(w, panes.List()) }

// extCreate is POST /api/ext {name, html}: a pane the user writes in the UI (full access).
func (s *server) extCreate(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, HTML string }
	if err := json.NewDecoder(io.LimitReader(r.Body, panes.MaxHTML+4096)).Decode(&in); err != nil {
		fail(w, err)
		return
	}
	if err := panes.Create(in.Name, in.HTML); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"name": in.Name})
}

// extDelete is DELETE /api/ext/{name}.
func (s *server) extDelete(w http.ResponseWriter, r *http.Request) {
	if err := panes.Delete(r.PathValue("name")); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Write([]byte("{}"))
}

// extGrant is PUT /api/ext/{name}/grant {allow}: lets an agent's pane act, or takes that back. Only this user
// action sets it, and it is bound to the HTML as it is now.
func (s *server) extGrant(w http.ResponseWriter, r *http.Request) {
	var in struct{ Allow bool }
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&in); err != nil {
		fail(w, err)
		return
	}
	if err := panes.Grant(r.PathValue("name"), in.Allow); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Write([]byte("{}"))
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
