package roles

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHarnessAgents(t *testing.T) {
	repo := t.TempDir()
	if err := exec.Command("git", "-C", repo, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "src")
	os.MkdirAll(sub, 0755)
	cases := []struct{ harness, file, content, name, prompt string }{
		{"claude", "reviewer.md", "---\nname: reviewer\ndescription: Reviews code\n---\nReview changes", "reviewer", "Review changes"},
		{"codex", "tester.toml", "name = 'tester'\ndescription = 'Runs tests'\ndeveloper_instructions = '''Run checks\nand report.'''", "tester", "Run checks\nand report."},
		{"opencode", "team/reviewer.md", "---\ndescription: Reviews changes\nmode: subagent\n---\nReview code", "team/reviewer", "Review code"},
	}
	for _, c := range cases {
		dir := DirFor(sub, c.harness)
		file := filepath.Join(dir, c.file)
		os.MkdirAll(filepath.Dir(file), 0755)
		os.WriteFile(file, []byte(c.content), 0644)
		defs := ListIn(sub, c.harness)
		if len(defs) != 1 || defs[0].Name != c.name || defs[0].Path != file || !strings.Contains(defs[0].Instructions, c.prompt) {
			t.Fatalf("%s: %+v", c.harness, defs)
		}
		if _, ok := GetIn(sub, c.harness, "../other"); ok {
			t.Fatal("path traversal accepted")
		}
	}
	if got := NamesIn(t.TempDir(), "claude"); len(got) != 0 {
		t.Fatalf("leaked: %v", got)
	}
}
