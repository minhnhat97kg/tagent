package workplace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrowserLifecycle(t *testing.T) {
	base := filepath.Join(t.TempDir(), "browser")
	b := Browser{Base: base}
	root, err := b.Create("/repo", "7")
	if err != nil || root != "/repo" {
		t.Fatalf("create: %q %v", root, err)
	}
	for _, dir := range []string{b.Profile("7"), b.Output("/repo", "7")} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("create missing %s: %v", dir, err)
		}
	}
	os.WriteFile(filepath.Join(b.Output("/repo", "7"), "shot.png"), []byte("x"), 0o600)
	if _, err := b.Create("/repo", "7"); err != nil {
		t.Fatalf("re-create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.Output("/repo", "7"), "shot.png")); err != nil {
		t.Fatalf("re-create lost captures: %v", err)
	}
	if notes := (Browser{Base: base, Keep: true}).Cleanup("/repo", "7"); notes != nil {
		t.Fatalf("keep: %v", notes)
	}
	if _, err := os.Stat(b.Profile("7")); err != nil {
		t.Fatalf("keep removed the profile: %v", err)
	}
	if notes := b.Cleanup("/repo", "7"); notes != nil {
		t.Fatalf("cleanup: %v", notes)
	}
	if _, err := os.Stat(filepath.Join(base, "7")); !os.IsNotExist(err) {
		t.Fatalf("cleanup kept the ticket dir: %v", err)
	}
}

func TestBrowserRejectsBadIDAndKeepsBase(t *testing.T) {
	base := filepath.Join(t.TempDir(), "browser")
	b := Browser{Base: base}
	os.MkdirAll(base, 0o700)
	for _, id := range []string{"", "a/b", "../x", "a.b"} {
		if _, err := b.Create("/repo", id); err == nil {
			t.Fatalf("create accepted %q", id)
		}
	}
	if notes := b.Cleanup("/repo", ""); notes != nil {
		t.Fatalf("empty id cleanup: %v", notes)
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("empty id removed base: %v", err)
	}
}
