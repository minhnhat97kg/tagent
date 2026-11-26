package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Load and List cache by mtime/size; a changed, deleted or broken file must not
// be served from the cache.
func TestLoadCacheInvalidates(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.yml")
	write := func(body string, mod time.Time) {
		t.Helper()
		if err := os.WriteFile(f, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Unix(1_000_000, 0)
	write("name: one\njobs:\n  build: {agent: x}\n", base)
	if w := Load(f); w.Name != "one" || w.Err != nil {
		t.Fatalf("first load: %+v", w)
	}
	// Different size, same mtime: size alone must invalidate.
	write("name: twotwo\njobs:\n  build: {agent: x}\n", base)
	if w := Load(f); w.Name != "twotwo" {
		t.Fatalf("size change served stale: %q", w.Name)
	}
	// A parse error is cached but a fixed file must replace it.
	write("name: [\n", base.Add(time.Second))
	if w := Load(f); w.Err == nil {
		t.Fatal("broken file parsed without error")
	}
	write("name: fixed\njobs:\n  build: {agent: x}\n", base.Add(2*time.Second))
	if w := Load(f); w.Err != nil || w.Name != "fixed" {
		t.Fatalf("fixed file served stale error: %+v", w)
	}
	// Deleted file: List must drop it (dir mtime changes).
	root := t.TempDir()
	wf := LocalDir(root)
	if err := os.MkdirAll(wf, 0700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(wf, "x.yml")
	if err := os.WriteFile(p, []byte("name: x\njobs:\n  build: {agent: x}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if n := len(List(root)); n != 1 {
		t.Fatalf("want 1 workflow, got %d", n)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	for _, w := range List(root) {
		if w.File == p {
			t.Fatal("deleted workflow still listed")
		}
	}
}
