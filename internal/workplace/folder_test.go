package workplace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderLifecycle(t *testing.T) {
	base := filepath.Join(t.TempDir(), "folder")
	f := Folder{Base: base}
	dir, err := f.Create("/repo", "7")
	if err != nil || dir != filepath.Join(base, "7") {
		t.Fatalf("create: %q %v", dir, err)
	}
	if b, _ := os.ReadFile(filepath.Join(base, ".repo")); string(b) != "/repo" {
		t.Fatalf(".repo = %q", b)
	}
	os.WriteFile(filepath.Join(dir, "note.md"), []byte("x"), 0o600)
	if _, err := f.Create("/repo", "7"); err != nil {
		t.Fatalf("re-create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "note.md")); err != nil {
		t.Fatalf("re-create lost files: %v", err)
	}
	if note, err := f.Gate("/repo", "7", "t"); note != "" || err != nil {
		t.Fatalf("gate: %q %v", note, err)
	}
	if notes := (Folder{Base: base, Keep: true}).Cleanup("/repo", "7"); notes != nil {
		t.Fatalf("keep: %v", notes)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("keep removed the folder: %v", err)
	}
	if notes := f.Cleanup("/repo", "7"); notes != nil {
		t.Fatalf("cleanup: %v", notes)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("cleanup kept the folder: %v", err)
	}
}

func TestFolderRejectsBadIDAndKeepsBase(t *testing.T) {
	base := filepath.Join(t.TempDir(), "folder")
	f := Folder{Base: base}
	os.MkdirAll(base, 0o700)
	for _, id := range []string{"", "a/b", "../x", "a.b"} {
		if _, err := f.Create("/repo", id); err == nil {
			t.Fatalf("create accepted %q", id)
		}
	}
	if notes := f.Cleanup("/repo", ""); notes != nil {
		t.Fatalf("empty id cleanup: %v", notes)
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("empty id removed base: %v", err)
	}
}
