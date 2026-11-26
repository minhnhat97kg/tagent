package roles

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestHarnessesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if !slices.Equal(HarnessNames(), builtinHarnesses) {
		t.Fatalf("no file: %v", HarnessNames())
	}
	path := filepath.Join(dir, "tagent", "harnesses.yml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(s string) {
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("harnesses:\n  - name: pi\n  - name: aider\n    args: [--yes, 'a b']\n")
	if got := HarnessNames(); !slices.Equal(got, []string{"pi", "aider"}) {
		t.Fatalf("names: %v", got)
	}
	if h, err := ResolveHarness("/usr/bin/aider"); err != nil || h.Command != "/usr/bin/aider" || !slices.Equal(h.Args, []string{"--yes", "a b"}) {
		t.Fatalf("args: %v %v", h, err)
	}
	write("harnesses:\n  - name: pi\n  - name: aider\n  - name: Claude API\n    command: claude\n    env: {B: x y, A_DIR: ~/.claude-api}\n")
	t.Setenv("HOME", "/h")
	h, err := ResolveHarness("Claude API")
	if err != nil || h.Command != "claude" || Harness("Claude API") != "claude" ||
		!slices.Equal(h.EnvAssignments(), []string{"A_DIR=/h/.claude-api", "B=x y"}) {
		t.Fatalf("alias: %v %v", h, err)
	}
	if !KnownHarness("aider") || !KnownHarness("claude") || KnownHarness("dev") {
		t.Fatal("KnownHarness")
	}
	write("harnesses:\n  - args: [x]\n")
	if _, err := ResolveHarness("claude"); err == nil {
		t.Fatal("want error for a nameless entry")
	}
	write("harnesses:\n  - name: pi\n    env: {'a b': x}\n")
	if _, err := ResolveHarness("pi"); err == nil {
		t.Fatal("want error for a bad env name")
	}
	if !slices.Equal(HarnessNames(), builtinHarnesses) {
		t.Fatalf("broken file: %v", HarnessNames())
	}
}
