package web

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tagent/internal/board"
)

// addWorkspace: POST {"Dir": "..."} adds a folder and answers with its main repo.
func (s *server) addWorkspace(w http.ResponseWriter, r *http.Request) {
	var o struct{ Dir string }
	if err := readBody(r, &o); err != nil {
		fail(w, err)
		return
	}
	root, err := board.AddWorkspace(o.Dir)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, map[string]string{"dir": root})
}

// removeWorkspace: DELETE ?dir= hides a workspace from the lists.
func (s *server) removeWorkspace(w http.ResponseWriter, r *http.Request) {
	if err := board.RemoveWorkspace(r.URL.Query().Get("dir")); err != nil {
		fail(w, err)
	}
}

// dirs lists the subfolders of ?path= (default $HOME) for the folder picker.
func (s *server) dirs(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fail(w, err)
			return
		}
		p = home
	}
	if !filepath.IsAbs(p) {
		fail(w, fmt.Errorf("%q: path must be absolute", p))
		return
	}
	p = filepath.Clean(p)
	entries, err := os.ReadDir(p)
	if err != nil {
		fail(w, err)
		return
	}
	names := []string{}
	for _, e := range entries {
		if st, err := os.Stat(filepath.Join(p, e.Name())); err == nil && st.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	parent := filepath.Dir(p)
	if parent == p {
		parent = ""
	}
	writeJSON(w, map[string]any{"path": p, "parent": parent, "dirs": names})
}
