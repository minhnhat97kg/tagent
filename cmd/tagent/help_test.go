package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpListsRepoAgents(t *testing.T) {
	repo := t.TempDir()
	if err := exec.Command("git", "-C", repo, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, ".claude", "agents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "reviewer.md"), []byte("Review changes"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{"codex", "opencode"} {
		agentDir := filepath.Join(repo, "."+harness, "agents")
		if err := os.MkdirAll(agentDir, 0755); err != nil {
			t.Fatal(err)
		}
		name, content := "reviewer.md", "Review changes"
		if harness == "codex" {
			name, content = "reviewer.toml", "name = 'reviewer'\ndescription = 'Review'\ndeveloper_instructions = 'Review changes'"
		}
		if err := os.WriteFile(filepath.Join(agentDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(repo)
	t.Setenv("TMUX", "") // the user's own tmux server may pick another default harness
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	var out, errOut bytes.Buffer
	if code := run([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("help code %d: %s", code, &errOut)
	}
	got := out.String()
	for _, want := range []string{"task add", "task retry id", "dispatch activity", "new --repo-agent", "Agents defined in this repo (", "  reviewer\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("help missing %q", want)
		}
	}
}
