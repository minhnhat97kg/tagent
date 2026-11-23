package roles

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := exec.Command("git", "-C", repo, "init", "-q").Run(); err != nil {
		t.Fatal(err)
	}
	return repo
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func find(defs []Definition, name string) (Definition, bool) {
	for _, d := range defs {
		if d.Name == name {
			return d, true
		}
	}
	return Definition{}, false
}

func TestLocalDefinitionsAreNotHarnessFiltered(t *testing.T) {
	repo := gitRepo(t)
	sub := filepath.Join(repo, "src")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(LocalDir(sub), "researcher.md"), "Investigate and report.\n")
	write(t, filepath.Join(DirFor(sub, "claude"), "reviewer.md"), "---\nname: reviewer\ndescription: Reviews code\n---\nReview changes")
	write(t, filepath.Join(DirFor(sub, "codex"), "tester.toml"), "name = 'tester'\ndescription = 'Runs tests'\ndeveloper_instructions = '''Run checks\nand report.'''")
	write(t, filepath.Join(DirFor(sub, "opencode"), "team/reviewer.md"), "---\ndescription: Reviews changes\nmode: subagent\n---\nReview code")

	defs := ListLocal(sub)
	for _, want := range []struct{ name, harness, prompt string }{
		{"researcher", "", "Investigate and report."},
		{"tester", "codex", "Run checks\nand report."},
		{"team/reviewer", "opencode", "Review code"},
		{"reviewer", "claude", "Review changes"},
	} {
		d, ok := find(defs, want.name)
		if !ok || d.Harness != want.harness || !strings.Contains(d.Instructions, want.prompt) {
			t.Fatalf("%s: %+v", want.name, defs)
		}
		if d.Native != (want.harness != "") {
			t.Fatalf("%s native: %+v", want.name, d)
		}
	}
	// The app never resolves a path outside the repo's agent names.
	if _, ok := GetIn(sub, "../other"); ok {
		t.Fatal("path traversal accepted")
	}
	if got := NamesIn(t.TempDir()); len(got) != 0 {
		t.Fatalf("leaked: %v", got)
	}
}

func TestAppOwnedInstructionsWinOverLegacy(t *testing.T) {
	repo := gitRepo(t)
	write(t, filepath.Join(DirFor(repo, "claude"), "researcher.md"), "legacy")
	write(t, filepath.Join(LocalPath(repo, "researcher")), "---\ndescription: Investigates\n---\napp owned")
	d, ok := GetIn(repo, "researcher")
	if !ok || d.Path != LocalPath(repo, "researcher") || d.Native || d.Instructions != "app owned" || d.Description != "Investigates" {
		t.Fatalf("researcher: %+v", d)
	}
}

func TestGlobalDefinitions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	write(t, filepath.Join(GlobalDir(), "planner.md"), "Plan.")
	write(t, filepath.Join(GlobalDir(), "claude", "gl.md"), "---\nname: gl\n---\nglobal legacy")
	write(t, filepath.Join(GlobalDir(), "profiles.toml"), "[planner]\nharness = 'pi'\n")
	defs := ListGlobal()
	if p, ok := find(defs, "planner"); !ok || p.Harness != "" || p.Instructions != "Plan." {
		t.Fatalf("planner: %+v", defs)
	}
	if gl, ok := find(defs, "gl"); !ok || gl.Harness != "claude" || gl.Native {
		t.Fatalf("global legacy must name its harness and stay non-native: %+v", defs)
	}
	if got := ProfileFor("planner"); got.Harness != "pi" {
		t.Fatalf("profile: %+v", got)
	}
}
