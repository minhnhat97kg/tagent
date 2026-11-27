package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListNativeReadOnlySources(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	for _, item := range []struct{ dir, source string }{
		{filepath.Join(home, ".claude", "skills", "user"), "claude-user"},
		{filepath.Join(root, ".claude", "skills", "project"), "claude-project"},
		{filepath.Join(home, ".codex", "skills", "codex"), "codex"},
		{filepath.Join(home, ".pi", "agent", "skills", "pi"), "pi"},
	} {
		if err := os.MkdirAll(item.dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(item.dir, "SKILL.md"), []byte("# skill"), 0444); err != nil {
			t.Fatal(err)
		}
	}
	got := ListNative(home, root)
	if len(got) != 4 {
		t.Fatalf("got %d skills: %#v", len(got), got)
	}
	for _, item := range got {
		if item.Source == "library" {
			t.Fatalf("native skill marked writable: %#v", item)
		}
		if _, err := os.Stat(filepath.Join(item.Path, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}
