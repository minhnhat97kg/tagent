package tasks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A non-repo dir (e.g. a pane at ~) must not fork git on every MainRoot call:
// project() asks per ticket x pane, so the miss path is the hot one.
func TestMainRootCachesNonRepoDir(t *testing.T) {
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "git.log")
	script := "#!/bin/sh\necho call >> '" + log + "'\nexit 128\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	for range 5 {
		if got := MainRoot(dir); got != dir {
			t.Fatalf("MainRoot(%q) = %q", dir, got)
		}
	}
	b, _ := os.ReadFile(log)
	if n := strings.Count(string(b), "call"); n != 1 {
		t.Fatalf("git ran %d times for one non-repo dir, want 1", n)
	}
}
