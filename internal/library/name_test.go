package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillNameCannotLeaveItsFolder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".tasks", "library", "skills", "bad")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: ../../escape\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := Load(root)
	if err != nil || len(l.Skills) != 0 || l.Invalid["skill:bad"] == nil {
		t.Fatalf("skills %v invalid %v err %v", l.Skills, l.Invalid, err)
	}
}
