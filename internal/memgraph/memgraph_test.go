package memgraph

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func newRepoGraph(t *testing.T) *Graph {
	t.Helper()
	g, err := Open(ScopeRepo, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func text(label, content string) Node {
	return Node{Label: label, Kind: "text", Content: content}
}

func mustNode(t *testing.T, g *Graph, actor string, n Node) Node {
	t.Helper()
	out, err := g.AddNode(actor, n)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func strp(s string) *string { return &s }

// TestHelperWriter is run as a subprocess by TestConcurrentWritersLoseNothing.
func TestHelperWriter(t *testing.T) {
	dir := os.Getenv("MEMGRAPH_HELPER_DIR")
	if dir == "" {
		t.Skip("helper only")
	}
	g, err := Open(ScopeRepo, dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 20 {
		if _, err := g.AddNode("user", text(fmt.Sprintf("p%d", i), "x")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConcurrentWritersLoseNothing(t *testing.T) {
	root := t.TempDir()
	g, _ := Open(ScopeRepo, root)
	var procs []*exec.Cmd
	for range 3 {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHelperWriter$")
		cmd.Env = append(os.Environ(), "MEMGRAPH_HELPER_DIR="+root)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		procs = append(procs, cmd)
	}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := g.AddNode("user", text(fmt.Sprintf("g%d", i), "y")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, p := range procs {
		if err := p.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	nodes, bad := loadAll[Node](filepath.Join(g.Dir, "nodes"))
	if len(nodes) != 80 || len(bad) != 0 {
		t.Fatalf("want 80 good nodes, got %d good, %d bad", len(nodes), len(bad))
	}
}

func TestConcurrentEditSameNodeSerialized(t *testing.T) {
	g := newRepoGraph(t)
	n := mustNode(t, g, "user", text("a", "b"))
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := g.EditNode("user", n.ID, NodePatch{Label: strp(fmt.Sprint("l", i))}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, _ := g.Get(n.ID)
	if got.Node.Rev != 21 {
		t.Fatalf("rev = %d, want 21", got.Node.Rev)
	}
}

func TestLockTimeout(t *testing.T) {
	g := newRepoGraph(t)
	if err := os.MkdirAll(g.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(filepath.Join(g.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	old := lockTimeout
	lockTimeout = 200 * time.Millisecond
	defer func() { lockTimeout = old }()
	_, err := g.AddNode("user", text("a", "b"))
	if err == nil || !strings.Contains(err.Error(), g.Dir) {
		t.Fatalf("want timeout error naming graph, got %v", err)
	}
	if ents, _ := os.ReadDir(filepath.Join(g.Dir, "nodes")); len(ents) != 0 {
		t.Fatal("nothing should be written")
	}
}

func TestWriteFailureKeepsOldRecord(t *testing.T) {
	g := newRepoGraph(t)
	n := mustNode(t, g, "user", text("old", "keep"))
	old := renameFn
	renameFn = func(string, string) error { return errors.New("disk full") }
	defer func() { renameFn = old }()
	if _, err := g.EditNode("user", n.ID, NodePatch{Label: strp("new")}); err == nil {
		t.Fatal("want error")
	}
	got, _ := g.Get(n.ID)
	if got.Node.Label != "old" {
		t.Fatalf("label = %q, old record was damaged", got.Node.Label)
	}
	ents, _ := os.ReadDir(filepath.Join(g.Dir, "nodes"))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatal("tmp file left behind")
		}
	}
}

func TestMissingGraphReadsEmpty(t *testing.T) {
	g := newRepoGraph(t)
	hits, err := g.Search("anything", 0)
	if err != nil || len(hits) != 0 {
		t.Fatalf("search: %v %v", hits, err)
	}
	if _, err := os.Stat(g.Dir); !os.IsNotExist(err) {
		t.Fatal("read must not create the graph dir")
	}
	if _, err := g.AddNode("user", text("a", "b")); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(g.Dir); st.Mode().Perm() != 0o700 {
		t.Fatalf("graph dir perm = %v", st.Mode().Perm())
	}
}

func TestCorruptRecordSkipped(t *testing.T) {
	g := newRepoGraph(t)
	mustNode(t, g, "user", text("good thing", "hello"))
	if err := writeAtomic(filepath.Join(g.Dir, "nodes", "bad.json"), []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	hits, _ := g.Search("good", 0)
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	rep, _ := g.Check()
	if len(rep.Corrupt) != 1 || rep.Corrupt[0] != "nodes/bad.json" {
		t.Fatalf("check corrupt = %v", rep.Corrupt)
	}
}

func TestSecondRunIdempotentLink(t *testing.T) {
	g := newRepoGraph(t)
	a := mustNode(t, g, "user", text("a", "1"))
	b := mustNode(t, g, "user", text("b", "2"))
	if a.ID == mustNode(t, g, "user", text("a", "1")).ID {
		t.Fatal("identical nodes must get distinct ids")
	}
	l1, err := g.AddLink("user", a.ID, b.ID, "uses")
	if err != nil {
		t.Fatal(err)
	}
	l2, _ := g.AddLink("user", a.ID, b.ID, "uses")
	if l1.ID != l2.ID {
		t.Fatal("identical link must be returned, not duplicated")
	}
}

func TestNodeRequiresLabelAndContent(t *testing.T) {
	g := newRepoGraph(t)
	if _, err := g.AddNode("user", Node{Kind: "text", Content: "x"}); err == nil {
		t.Fatal("empty label accepted")
	}
	if _, err := g.AddNode("user", Node{Label: "a", Kind: "text"}); err == nil {
		t.Fatal("empty content accepted")
	}
}

func TestLinkNeedsExistingNodes(t *testing.T) {
	g := newRepoGraph(t)
	a := mustNode(t, g, "user", text("a", "1"))
	if _, err := g.AddLink("user", a.ID, "missing", "x"); err == nil {
		t.Fatal("link to missing node accepted")
	}
}

func TestLinkRequiresLabel(t *testing.T) {
	g := newRepoGraph(t)
	a := mustNode(t, g, "user", text("a", "1"))
	b := mustNode(t, g, "user", text("b", "2"))
	if _, err := g.AddLink("user", a.ID, b.ID, " "); err == nil {
		t.Fatal("empty link label accepted")
	}
}

func TestStructuredKindsValidated(t *testing.T) {
	g := newRepoGraph(t)
	if _, err := g.AddNode("user", Node{Label: "j", Kind: "json", Content: "{"}); err == nil {
		t.Fatal("bad json accepted")
	}
	if _, err := g.AddNode("user", Node{Label: "y", Kind: "yaml", Content: "a: [b"}); err == nil {
		t.Fatal("bad yaml accepted")
	}
	if _, err := g.AddNode("user", Node{Label: "y", Kind: "yaml", Content: "a: 1"}); err != nil {
		t.Fatal(err)
	}
}

func TestFileRefRejectsEscape(t *testing.T) {
	g := newRepoGraph(t)
	for _, ref := range []string{"../x", "/etc/passwd", "a/../../x"} {
		if _, err := g.AddNode("user", Node{Label: "f", Kind: "file", Ref: ref}); err == nil {
			t.Fatalf("ref %q accepted", ref)
		}
	}
}

func TestFileRefNotCopied(t *testing.T) {
	g := newRepoGraph(t)
	if err := os.WriteFile(filepath.Join(g.Root, "main.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	n := mustNode(t, g, "user", Node{Label: "main", Kind: "file", Ref: "main.go"})
	if n.Content != "" {
		t.Fatal("file content was copied")
	}
	if _, err := os.Stat(filepath.Join(g.Dir, "blobs")); !os.IsNotExist(err) {
		t.Fatal("file ref created blobs")
	}
}

func TestImageValidation(t *testing.T) {
	g := newRepoGraph(t)
	dir := t.TempDir()
	png := filepath.Join(dir, "a.png")
	os.WriteFile(png, []byte("\x89PNG\r\n\x1a\nrest"), 0o600)
	n := mustNode(t, g, "user", Node{Label: "pic", Kind: "image", Ref: png})
	if !strings.HasPrefix(n.Ref, "blobs/") || !strings.HasSuffix(n.Ref, ".png") {
		t.Fatalf("ref = %q", n.Ref)
	}
	txt := filepath.Join(dir, "b.png")
	os.WriteFile(txt, []byte("not an image"), 0o600)
	if _, err := g.AddNode("user", Node{Label: "x", Kind: "image", Ref: txt}); err == nil {
		t.Fatal("non-image accepted")
	}
	big := filepath.Join(dir, "big.png")
	os.WriteFile(big, append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, maxImage)...), 0o600)
	if _, err := g.AddNode("user", Node{Label: "x", Kind: "image", Ref: big}); err == nil {
		t.Fatal("oversized image accepted")
	}
}

func TestIdTraversalRejected(t *testing.T) {
	g := newRepoGraph(t)
	for _, id := range []string{"../x", "a/b", "", ".."} {
		if _, err := g.Get(id); err == nil {
			t.Fatalf("id %q accepted", id)
		}
	}
}

func TestAgentNodeAuthor(t *testing.T) {
	g := newRepoGraph(t)
	n := mustNode(t, g, "pm", text("a", "b"))
	if n.Author != "pm" || n.Rev != 1 {
		t.Fatalf("author=%q rev=%d", n.Author, n.Rev)
	}
}

func TestAgentEditsOwnNode(t *testing.T) {
	g := newRepoGraph(t)
	n := mustNode(t, g, "pm", text("a", "b"))
	out, err := g.EditNode("pm", n.ID, NodePatch{Content: strp("c")})
	if err != nil || out.Rev != 2 {
		t.Fatalf("err=%v rev=%d", err, out.Rev)
	}
}

func TestAgentCannotEditForeign(t *testing.T) {
	g := newRepoGraph(t)
	n := mustNode(t, g, "user", text("a", "b"))
	if _, err := g.EditNode("pm", n.ID, NodePatch{Label: strp("x")}); err == nil {
		t.Fatal("agent edited user node")
	}
	m := mustNode(t, g, "dev", text("c", "d"))
	if _, err := g.EditNode("pm", m.ID, NodePatch{Label: strp("x")}); err == nil {
		t.Fatal("agent edited another role's node")
	}
	got, _ := g.Get(n.ID)
	if got.Node.Label != "a" || got.Node.Rev != 1 {
		t.Fatal("record changed")
	}
}

func TestUserEditTakesOwnership(t *testing.T) {
	g := newRepoGraph(t)
	n := mustNode(t, g, "pm", text("a", "b"))
	l := mustLink(t, g, n, mustNode(t, g, "pm", text("c", "d")), "x", "pm")
	out, err := g.EditNode("user", n.ID, NodePatch{Label: strp("u")})
	if err != nil || out.Author != "user" {
		t.Fatalf("err=%v author=%q", err, out.Author)
	}
	if _, err := g.EditNode("pm", n.ID, NodePatch{Label: strp("again")}); err == nil {
		t.Fatal("agent could edit after user took ownership")
	}
	if _, err := g.EditLink("user", l.ID, "relabelled"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.EditLink("pm", l.ID, "x"); err == nil {
		t.Fatal("agent could edit link after user took ownership")
	}
}

func mustLink(t *testing.T, g *Graph, a, b Node, label, actor string) Link {
	t.Helper()
	l, err := g.AddLink(actor, a.ID, b.ID, label)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestUserDeleteRemovesLinks(t *testing.T) {
	g := newRepoGraph(t)
	a := mustNode(t, g, "user", text("a", "1"))
	b := mustNode(t, g, "user", text("b", "2"))
	l := mustLink(t, g, a, b, "x", "user")
	if err := g.DeleteNode("user", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := g.readLink(l.ID); err == nil {
		t.Fatal("link survived node delete")
	}
}

func TestAgentCannotDelete(t *testing.T) {
	g := newRepoGraph(t)
	a := mustNode(t, g, "pm", text("a", "1"))
	b := mustNode(t, g, "pm", text("b", "2"))
	l := mustLink(t, g, a, b, "x", "pm")
	if err := g.DeleteNode("pm", a.ID); err == nil {
		t.Fatal("agent deleted node")
	}
	if err := g.DeleteLink("pm", l.ID); err == nil {
		t.Fatal("agent deleted link")
	}
}

func TestSearchNoMatch(t *testing.T) {
	g := newRepoGraph(t)
	mustNode(t, g, "user", text("a", "1"))
	hits, err := g.Search("zzz", 0)
	if err != nil || len(hits) != 0 {
		t.Fatalf("hits=%v err=%v", hits, err)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	g := newRepoGraph(t)
	if _, err := g.Search("  ", 0); err == nil {
		t.Fatal("empty query accepted")
	}
}

func TestSearchLimitEnforced(t *testing.T) {
	g := newRepoGraph(t)
	for i := range 60 {
		mustNode(t, g, "user", text(fmt.Sprint("item ", i), "same"))
	}
	hits, _ := g.Search("item", 999)
	if len(hits) != 50 {
		t.Fatalf("got %d hits, want 50", len(hits))
	}
	hits, _ = g.Search("item", 0)
	if len(hits) != 10 {
		t.Fatalf("default limit gave %d", len(hits))
	}
}

func TestRelatedClampedAndCycle(t *testing.T) {
	g := newRepoGraph(t)
	a := mustNode(t, g, "user", text("a", "1"))
	b := mustNode(t, g, "user", text("b", "2"))
	c := mustNode(t, g, "user", text("c", "3"))
	mustLink(t, g, a, b, "x", "user")
	mustLink(t, g, b, c, "y", "user")
	mustLink(t, g, c, a, "z", "user")
	steps, err := g.Related(a.ID, 99, 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 {
		t.Fatalf("cycle gave %d steps, want 2 unique nodes", len(steps))
	}
	steps, _ = g.Related(a.ID, 1, 0)
	if len(steps) != 2 || steps[0].Depth != 1 {
		t.Fatalf("depth 1 gave %d steps", len(steps))
	}
}

func TestBrokenRefReported(t *testing.T) {
	g := newRepoGraph(t)
	os.WriteFile(filepath.Join(g.Root, "gone.go"), []byte("x"), 0o600)
	n := mustNode(t, g, "user", Node{Label: "gone", Kind: "file", Ref: "gone.go"})
	os.Remove(filepath.Join(g.Root, "gone.go"))
	hit, err := g.Get(n.ID)
	if err != nil || !hit.BrokenRef {
		t.Fatalf("broken=%v err=%v", hit.BrokenRef, err)
	}
	rep, _ := g.Check()
	if len(rep.BrokenRefs) != 1 {
		t.Fatalf("check broken refs = %d", len(rep.BrokenRefs))
	}
	after, _ := g.Get(n.ID)
	if after.Node.Rev != 1 {
		t.Fatal("broken node was changed")
	}
}

func TestSharedGraphDefaultDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	g, err := Open(ScopeShared, "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "tagent", "graph")
	if g.Dir != want {
		t.Fatalf("dir = %s, want %s", g.Dir, want)
	}
}

func TestOpenRejectsBadScope(t *testing.T) {
	if _, err := Open("both", "/x"); err == nil {
		t.Fatal("bad scope accepted")
	}
	if _, err := Open(ScopeRepo, ""); err == nil {
		t.Fatal("repo scope without root accepted")
	}
}
