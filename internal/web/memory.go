package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"tagent/internal/memgraph"
)

type memoryBody struct {
	Scope string        `json:"scope"`
	Node  memgraph.Node `json:"node"`
	ID    string        `json:"id"`
	From  string        `json:"from"`
	To    string        `json:"to"`
	Label string        `json:"label"`
}

func (s *server) memory(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		http.Error(w, "scope is required", 400)
		return
	}
	root := ""
	if scope == memgraph.ScopeRepo {
		root = s.projectDir(r)
		if root == "" {
			http.Error(w, "repo scope is unavailable", 400)
			return
		}
	}
	g, err := memgraph.Open(scope, root)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query().Get("q")
		hits, e := g.Search(q, 1000)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		report, e := g.Check()
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		links := []memgraph.Link{}
		entries, _ := os.ReadDir(filepath.Join(g.Dir, "links"))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			raw, readErr := os.ReadFile(filepath.Join(g.Dir, "links", entry.Name()))
			var link memgraph.Link
			if readErr == nil && json.Unmarshal(raw, &link) == nil {
				links = append(links, link)
			}
		}
		writeJSON(w, map[string]any{"nodes": hits, "links": links, "report": report})
	case http.MethodPost:
		var b memoryBody
		if readBody(r, &b) != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		if b.ID != "" {
			if b.From != "" {
				l, e := g.AddLink("user", b.From, b.To, b.Label)
				if e != nil {
					fail(w, e)
					return
				}
				writeJSON(w, l)
				return
			}
			n, e := g.EditNode("user", b.ID, memgraph.NodePatch{Label: &b.Node.Label, Content: &b.Node.Content, Ref: &b.Node.Ref, Lang: &b.Node.Lang})
			if e != nil {
				fail(w, e)
				return
			}
			writeJSON(w, n)
			return
		}
		n, e := g.AddNode("user", b.Node)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, n)
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		kind := r.URL.Query().Get("kind")
		if kind == "link" {
			err = g.DeleteLink("user", id)
		} else {
			err = g.DeleteNode("user", id)
		}
		if err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(204)
	default:
		http.Error(w, "method not allowed", 405)
	}
}
