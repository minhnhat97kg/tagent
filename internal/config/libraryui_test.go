package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNextSkillPathCreatesDistinctEntries(t *testing.T) {
	root := t.TempDir()
	first := nextSkillPath(root)
	if err := os.MkdirAll(filepath.Dir(first), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("---\nname: skill-1\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	second := nextSkillPath(root)
	if first == second {
		t.Fatalf("new skill path reused %q", first)
	}
	if filepath.Base(filepath.Dir(first)) != "skill-1" || filepath.Base(filepath.Dir(second)) != "skill-2" {
		t.Fatalf("unexpected paths: %q, %q", first, second)
	}
}
