// Package memgraph stores a knowledge graph of nodes and links as one JSON file
// per record. Two scopes exist: a repo graph (<repo>/.tasks/graph) and one
// shared graph under the tagent config dir.
package memgraph

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"tagent/internal/apppath"

	"gopkg.in/yaml.v3"
)

const (
	ScopeRepo   = "repo"
	ScopeShared = "shared"

	userActor   = "user"
	maxText     = 256 << 10
	maxImage    = 5 << 20
	previewSize = 2 << 10
)

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Graph is one scope's storage directory.
type Graph struct {
	Scope string
	Dir   string
	Root  string // repo root for relative file refs; empty for shared
}

type Node struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Kind    string    `json:"kind"`
	Lang    string    `json:"lang,omitempty"`
	Content string    `json:"content,omitempty"`
	Ref     string    `json:"ref,omitempty"`
	Tags    []string  `json:"tags,omitempty"`
	Author  string    `json:"author"`
	Rev     int       `json:"rev"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

type Link struct {
	ID      string    `json:"id"`
	From    string    `json:"from"`
	To      string    `json:"to"`
	Label   string    `json:"label"`
	Author  string    `json:"author"`
	Rev     int       `json:"rev"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
}

// NodePatch holds the fields an edit may change; nil means unchanged.
type NodePatch struct {
	Label, Content, Ref, Lang *string
	Tags                      *[]string
}

// Hit is a node returned by search or show.
type Hit struct {
	Scope     string `json:"scope"`
	Node      Node   `json:"node"`
	BrokenRef bool   `json:"broken_ref,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Step is one node reached by Related, with the link used to reach it.
type Step struct {
	Depth int  `json:"depth"`
	Via   Link `json:"via"`
	Node  Hit  `json:"node"`
}

// Report is the output of Check.
type Report struct {
	Scope        string   `json:"scope"`
	Corrupt      []string `json:"corrupt,omitempty"`
	BrokenRefs   []Hit    `json:"broken_refs,omitempty"`
	DanglingLink []string `json:"dangling_links,omitempty"`
}

// renameFn is swapped in tests to simulate a failed write.
var renameFn = os.Rename

// Open returns the graph for a scope. repoRoot is required for ScopeRepo.
func Open(scope, repoRoot string) (*Graph, error) {
	switch scope {
	case ScopeRepo:
		if repoRoot == "" {
			return nil, errors.New("repo graph needs a repo root")
		}
		return &Graph{Scope: scope, Dir: filepath.Join(repoRoot, ".tasks", "graph"), Root: repoRoot}, nil
	case ScopeShared:
		return &Graph{Scope: scope, Dir: filepath.Join(apppath.ConfigDir(), "graph")}, nil
	}
	return nil, fmt.Errorf("unknown scope %q (want repo or shared)", scope)
}

func checkID(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("invalid id %q", id)
	}
	return nil
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (g *Graph) nodePath(id string) string { return filepath.Join(g.Dir, "nodes", id+".json") }
func (g *Graph) linkPath(id string) string { return filepath.Join(g.Dir, "links", id+".json") }

// writeAtomic writes via a temp file and rename, so readers never see a partial file.
func writeAtomic(path string, data []byte) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return renameFn(tmp.Name(), path)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(b, '\n'))
}

// loadAll reads every *.json record in dir. A missing dir is empty; corrupt files are listed, not fatal.
func loadAll[T any](dir string) ([]T, []string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	var out []T
	var bad []string
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		var v T
		if err != nil || json.Unmarshal(b, &v) != nil {
			bad = append(bad, e.Name())
			continue
		}
		out = append(out, v)
	}
	return out, bad
}

func (g *Graph) readNode(id string) (Node, error) {
	var n Node
	if err := checkID(id); err != nil {
		return n, err
	}
	b, err := os.ReadFile(g.nodePath(id))
	if os.IsNotExist(err) {
		return n, fmt.Errorf("no node %s in %s graph", id, g.Scope)
	}
	if err != nil {
		return n, err
	}
	if err := json.Unmarshal(b, &n); err != nil {
		return n, fmt.Errorf("node %s is corrupt: %w", id, err)
	}
	return n, nil
}

func (g *Graph) readLink(id string) (Link, error) {
	var l Link
	if err := checkID(id); err != nil {
		return l, err
	}
	b, err := os.ReadFile(g.linkPath(id))
	if os.IsNotExist(err) {
		return l, fmt.Errorf("no link %s in %s graph", id, g.Scope)
	}
	if err != nil {
		return l, err
	}
	if err := json.Unmarshal(b, &l); err != nil {
		return l, fmt.Errorf("link %s is corrupt: %w", id, err)
	}
	return l, nil
}

// checkOwner: the user may change anything; an agent only what it created.
func checkOwner(actor, author string) error {
	if actor == userActor || actor == author {
		return nil
	}
	return fmt.Errorf("%s cannot edit a record owned by %s", actor, author)
}

func (g *Graph) checkRef(ref string) error {
	if ref == "" {
		return errors.New("file reference needs a path")
	}
	if g.Scope == ScopeShared {
		if !filepath.IsAbs(ref) {
			return errors.New("shared graph file references need an absolute path")
		}
		return nil
	}
	c := filepath.Clean(ref)
	if filepath.IsAbs(c) || c == ".." || strings.HasPrefix(c, ".."+string(filepath.Separator)) {
		return fmt.Errorf("file reference %q escapes the repo", ref)
	}
	return nil
}

func validateNode(g *Graph, n Node) error {
	if strings.TrimSpace(n.Label) == "" {
		return errors.New("label is required")
	}
	switch n.Kind {
	case "text", "markdown", "code", "json", "yaml":
		if strings.TrimSpace(n.Content) == "" {
			return fmt.Errorf("content is required for kind %s", n.Kind)
		}
		if len(n.Content) > maxText {
			return fmt.Errorf("content exceeds %d bytes", maxText)
		}
		if n.Kind == "json" && !json.Valid([]byte(n.Content)) {
			return errors.New("content is not valid JSON")
		}
		if n.Kind == "yaml" {
			var v any
			if err := yaml.Unmarshal([]byte(n.Content), &v); err != nil {
				return fmt.Errorf("content is not valid YAML: %w", err)
			}
		}
	case "url":
		u, err := url.Parse(strings.TrimSpace(n.Content))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("url content must be an http(s) URL")
		}
	case "file":
		return g.checkRef(n.Ref)
	case "image":
		if n.Ref == "" {
			return errors.New("image needs a source file")
		}
	default:
		return fmt.Errorf("unknown kind %q", n.Kind)
	}
	return nil
}

// storeBlob copies an image into blobs/<sha256>.<ext> and returns its relative name.
func (g *Graph) storeBlob(src string) (string, error) {
	st, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if st.Size() > maxImage {
		return "", fmt.Errorf("image exceeds %d bytes", maxImage)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	var ext string
	switch {
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		ext = "png"
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		ext = "jpg"
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		ext = "gif"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		ext = "webp"
	default:
		return "", errors.New("not a png, jpg, gif or webp image")
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + "." + ext
	dst := filepath.Join(g.Dir, "blobs", name)
	if _, err := os.Stat(dst); err == nil {
		return "blobs/" + name, nil
	}
	if err := writeAtomic(dst, data); err != nil {
		return "", err
	}
	return "blobs/" + name, nil
}

// Broken reports whether a file or image node points at something missing.
func (g *Graph) Broken(n Node) bool {
	switch n.Kind {
	case "file":
		p := n.Ref
		if !filepath.IsAbs(p) {
			p = filepath.Join(g.Root, p)
		}
		_, err := os.Stat(p)
		return err != nil
	case "image":
		_, err := os.Stat(filepath.Join(g.Dir, n.Ref))
		return err != nil
	}
	return false
}

// AddNode creates a node owned by actor.
func (g *Graph) AddNode(actor string, n Node) (Node, error) {
	if actor == "" {
		return n, errors.New("actor is required")
	}
	if n.Kind == "image" {
		ref, err := g.storeBlob(n.Ref)
		if err != nil {
			return n, err
		}
		n.Ref = ref
	}
	if n.Kind == "file" && g.Scope != ScopeShared {
		n.Ref = filepath.Clean(n.Ref)
	}
	if err := validateNode(g, n); err != nil {
		return n, err
	}
	release, err := g.lock()
	if err != nil {
		return n, err
	}
	defer release()
	id, err := newID()
	if err != nil {
		return n, err
	}
	now := time.Now().UTC()
	n.ID, n.Author, n.Rev, n.Created, n.Updated = id, actor, 1, now, now
	return n, writeJSON(g.nodePath(id), n)
}

// EditNode applies p to a node. Agents may edit only what they own; a user edit transfers ownership to the user.
func (g *Graph) EditNode(actor, id string, p NodePatch) (Node, error) {
	release, err := g.lock()
	if err != nil {
		return Node{}, err
	}
	defer release()
	n, err := g.readNode(id)
	if err != nil {
		return n, err
	}
	if err := checkOwner(actor, n.Author); err != nil {
		return n, err
	}
	if p.Label != nil {
		n.Label = *p.Label
	}
	if p.Content != nil {
		n.Content = *p.Content
	}
	if p.Lang != nil {
		n.Lang = *p.Lang
	}
	if p.Tags != nil {
		n.Tags = *p.Tags
	}
	if p.Ref != nil {
		if n.Kind == "image" {
			return n, errors.New("image source cannot change; add a new node")
		}
		n.Ref = *p.Ref
		if n.Kind == "file" {
			n.Ref = filepath.Clean(n.Ref)
		}
	}
	if err := validateNode(g, n); err != nil {
		return n, err
	}
	if actor == userActor {
		n.Author = userActor
	}
	n.Rev++
	n.Updated = time.Now().UTC()
	return n, writeJSON(g.nodePath(id), n)
}

// AddLink joins two existing nodes. An identical (from, to, label) link is returned as-is.
func (g *Graph) AddLink(actor, from, to, label string) (Link, error) {
	if actor == "" {
		return Link{}, errors.New("actor is required")
	}
	if strings.TrimSpace(label) == "" {
		return Link{}, errors.New("link label is required")
	}
	release, err := g.lock()
	if err != nil {
		return Link{}, err
	}
	defer release()
	if _, err := g.readNode(from); err != nil {
		return Link{}, err
	}
	if _, err := g.readNode(to); err != nil {
		return Link{}, err
	}
	links, _ := loadAll[Link](filepath.Join(g.Dir, "links"))
	for _, l := range links {
		if l.From == from && l.To == to && l.Label == label {
			return l, nil
		}
	}
	id, err := newID()
	if err != nil {
		return Link{}, err
	}
	now := time.Now().UTC()
	l := Link{ID: id, From: from, To: to, Label: label, Author: actor, Rev: 1, Created: now, Updated: now}
	return l, writeJSON(g.linkPath(id), l)
}

// EditLink changes a link's label under the same ownership rule as nodes.
func (g *Graph) EditLink(actor, id, label string) (Link, error) {
	if strings.TrimSpace(label) == "" {
		return Link{}, errors.New("link label is required")
	}
	release, err := g.lock()
	if err != nil {
		return Link{}, err
	}
	defer release()
	l, err := g.readLink(id)
	if err != nil {
		return l, err
	}
	if err := checkOwner(actor, l.Author); err != nil {
		return l, err
	}
	l.Label = label
	if actor == userActor {
		l.Author = userActor
	}
	l.Rev++
	l.Updated = time.Now().UTC()
	return l, writeJSON(g.linkPath(id), l)
}

// DeleteNode removes a node and its links. Only the user may delete.
func (g *Graph) DeleteNode(actor, id string) error {
	if actor != userActor {
		return fmt.Errorf("%s cannot delete; only the user can", actor)
	}
	release, err := g.lock()
	if err != nil {
		return err
	}
	defer release()
	if _, err := g.readNode(id); err != nil {
		return err
	}
	links, _ := loadAll[Link](filepath.Join(g.Dir, "links"))
	for _, l := range links {
		if l.From == id || l.To == id {
			if err := os.Remove(g.linkPath(l.ID)); err != nil {
				return err
			}
		}
	}
	return os.Remove(g.nodePath(id))
}

// DeleteLink removes a link. Only the user may delete.
func (g *Graph) DeleteLink(actor, id string) error {
	if actor != userActor {
		return fmt.Errorf("%s cannot delete; only the user can", actor)
	}
	release, err := g.lock()
	if err != nil {
		return err
	}
	defer release()
	if _, err := g.readLink(id); err != nil {
		return err
	}
	return os.Remove(g.linkPath(id))
}

// Get returns a node with its broken-ref flag.
func (g *Graph) Get(id string) (Hit, error) {
	n, err := g.readNode(id)
	if err != nil {
		return Hit{}, err
	}
	return Hit{Scope: g.Scope, Node: n, BrokenRef: g.Broken(n)}, nil
}

// Search returns nodes whose label, content, tags or ref contain every query word.
func (g *Graph) Search(query string, limit int) ([]Hit, error) {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil, errors.New("search query is empty")
	}
	limit = clamp(limit, 10, 50)
	nodes, _ := loadAll[Node](filepath.Join(g.Dir, "nodes"))
	var out []Hit
	for _, n := range nodes {
		if len(out) >= limit {
			break
		}
		hay := strings.ToLower(n.Label + "\n" + n.Content + "\n" + strings.Join(n.Tags, " ") + "\n" + n.Ref)
		match := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		h := Hit{Scope: g.Scope, Node: n, BrokenRef: g.Broken(n)}
		if len(h.Node.Content) > previewSize {
			h.Node.Content = h.Node.Content[:previewSize]
			h.Truncated = true
		}
		out = append(out, h)
	}
	return out, nil
}

// Related walks links breadth-first from id. Depth is clamped to 1..3, limit to 1..50; each node appears once.
func (g *Graph) Related(id string, depth, limit int) ([]Step, error) {
	if _, err := g.readNode(id); err != nil {
		return nil, err
	}
	depth = clamp(depth, 1, 3)
	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 50)
	nodes, _ := loadAll[Node](filepath.Join(g.Dir, "nodes"))
	links, _ := loadAll[Link](filepath.Join(g.Dir, "links"))
	byID := make(map[string]Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	seen := map[string]bool{id: true}
	type item struct {
		id    string
		depth int
	}
	queue := []item{{id, 0}}
	var out []Step
	for len(queue) > 0 && len(out) < limit {
		cur := queue[0]
		queue = queue[1:]
		if cur.depth >= depth {
			continue
		}
		for _, l := range links {
			other := ""
			switch cur.id {
			case l.From:
				other = l.To
			case l.To:
				other = l.From
			default:
				continue
			}
			n, ok := byID[other]
			if !ok || seen[other] || len(out) >= limit {
				continue
			}
			seen[other] = true
			out = append(out, Step{Depth: cur.depth + 1, Via: l, Node: Hit{Scope: g.Scope, Node: n, BrokenRef: g.Broken(n)}})
			queue = append(queue, item{other, cur.depth + 1})
		}
	}
	return out, nil
}

// Check lists corrupt records, broken file/image refs and links to missing nodes. It changes nothing.
func (g *Graph) Check() (Report, error) {
	rep := Report{Scope: g.Scope}
	nodes, badN := loadAll[Node](filepath.Join(g.Dir, "nodes"))
	links, badL := loadAll[Link](filepath.Join(g.Dir, "links"))
	for _, b := range badN {
		rep.Corrupt = append(rep.Corrupt, "nodes/"+b)
	}
	for _, b := range badL {
		rep.Corrupt = append(rep.Corrupt, "links/"+b)
	}
	have := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		have[n.ID] = true
		if g.Broken(n) {
			rep.BrokenRefs = append(rep.BrokenRefs, Hit{Scope: g.Scope, Node: n, BrokenRef: true})
		}
	}
	for _, l := range links {
		if !have[l.From] || !have[l.To] {
			rep.DanglingLink = append(rep.DanglingLink, l.ID)
		}
	}
	return rep, nil
}

func clamp(v, lo, hi int) int {
	if v == 0 {
		return lo
	}
	return min(max(v, lo), hi)
}
