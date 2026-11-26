package panes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestPutCreatesAndOnlyCreatorChanges(t *testing.T) {
	setup(t)
	if err := Put("bot", "<p>1</p>", "dev-1"); err != nil {
		t.Fatal(err)
	}
	if got, ok := Get("bot"); !ok || !got.Agent || got.Actions {
		t.Fatalf("new: %+v %v", got, ok)
	}
	if err := Put("bot", "<p>2</p>", "dev-1"); err != nil {
		t.Fatalf("creator rewrites: %v", err)
	}
	if b, _ := os.ReadFile(File("bot")); string(b) != "<p>2</p>" {
		t.Fatalf("rewrite: %q", b)
	}
	if err := Put("bot", "<p>3</p>", "dev-2"); err == nil {
		t.Fatal("another agent overwrote the pane")
	}
	// The user's panes: files without a sidecar, or made in the UI.
	os.WriteFile(filepath.Join(Dir(), "hand.html"), []byte("mine"), 0o644)
	if err := Put("hand", "x", "dev-1"); err == nil {
		t.Fatal("agent overwrote a user pane")
	}
	if err := Create("ui", "mine"); err != nil {
		t.Fatal(err)
	}
	if err := Put("ui", "x", "dev-1"); err == nil {
		t.Fatal("agent overwrote a UI pane")
	}
	if err := Create("ui", "again"); err == nil {
		t.Fatal("create replaced a pane")
	}
	if b, _ := os.ReadFile(File("hand")); string(b) != "mine" {
		t.Fatalf("user pane changed: %q", b)
	}
}

func TestNamesSizeAndPaths(t *testing.T) {
	setup(t)
	for _, name := range []string{"", "../x", "a/b", `a\b`, "a.b", ".hidden", "-x", "a b", "x.html", strings.Repeat("a", 65), "a\x00b", "%2e%2e"} {
		if err := Put(name, "<p>x</p>", "dev"); err == nil {
			t.Fatalf("name %q accepted", name)
		}
		if err := Create(name, "<p>x</p>"); err == nil {
			t.Fatalf("name %q accepted by Create", name)
		}
	}
	if err := Put("big", strings.Repeat("a", MaxHTML+1), "dev"); err == nil {
		t.Fatal("over the limit accepted")
	}
	if err := Put("big", strings.Repeat("a", MaxHTML), "dev"); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	if err := Put("empty", " \n", "dev"); err == nil {
		t.Fatal("empty accepted")
	}
	// Only the ui folder gains files, and no temp file is left.
	entries, _ := os.ReadDir(filepath.Dir(Dir()))
	if len(entries) != 1 || entries[0].Name() != "ui" {
		t.Fatalf("config dir: %v", entries)
	}
	files, _ := os.ReadDir(Dir())
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".tmp") {
			t.Fatalf("temp file left: %s", f.Name())
		}
	}
}

func TestPutDoesNotWriteThroughSymlink(t *testing.T) {
	setup(t)
	os.MkdirAll(Dir(), 0o755)
	target := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(target, []byte("keep"), 0o644)
	os.Symlink(target, filepath.Join(Dir(), "evil.meta.json"))
	if err := Put("evil", "<p>x</p>", "dev"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Fatalf("wrote through a symlink: %q", b)
	}
}

func TestGrantIsBoundToTheHash(t *testing.T) {
	setup(t)
	Put("bot", "<p>1</p>", "dev")
	if err := Grant("bot", true); err != nil {
		t.Fatal(err)
	}
	if got, _ := Get("bot"); !got.Actions {
		t.Fatal("grant not seen")
	}
	Put("bot", "<p>2</p>", "dev")
	if got, _ := Get("bot"); got.Actions {
		t.Fatal("rewrite kept the grant")
	}
	// Even a grant left in the sidecar does not carry to other HTML, and the old HTML back does not revive it... only a new grant does.
	Grant("bot", true)
	os.WriteFile(File("bot"), []byte("<p>3</p>"), 0o644)
	if got, _ := Get("bot"); got.Actions {
		t.Fatal("edit kept the grant")
	}
	Create("mine", "x")
	if err := Grant("mine", true); err == nil {
		t.Fatal("granted a user pane")
	}
	if err := Delete("bot"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(metaFile("bot")); err == nil {
		t.Fatal("sidecar left after delete")
	}
	// A pane made again under that name starts read-only.
	Put("bot", "<p>1</p>", "dev")
	if got, _ := Get("bot"); got.Actions {
		t.Fatal("recreated pane has a grant")
	}
}
