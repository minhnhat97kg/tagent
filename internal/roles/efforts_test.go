package roles

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestHarnessEffortsParse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "tagent", "harnesses.yml")
	os.MkdirAll(filepath.Dir(path), 0o755)
	write := func(s string) {
		if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Absent: the built-in levels of the kind. Empty: none. Custom: exactly the list.
	write("harnesses:\n  - name: claude\n  - name: nothing\n    command: opencode\n    efforts: []\n  - name: deep\n    command: claude\n    efforts: [fast, deep]\n")
	if got := Efforts("claude"); !slices.Equal(got, kindEfforts("claude")) {
		t.Fatalf("absent: %v", got)
	}
	if got := Efforts("nothing"); got == nil || len(got) != 0 {
		t.Fatalf("empty: %v", got)
	}
	if got := Efforts("deep"); !slices.Equal(got, []string{"fast", "deep"}) {
		t.Fatalf("custom: %v", got)
	}
	if got := EffortMap()["nothing"]; got == nil {
		t.Fatal("EffortMap must not serve null")
	}
	if !KnownEffort("fast") || KnownEffort("ultra") {
		t.Fatal("KnownEffort")
	}
	// Custom levels on a kind that cannot pass them fail the whole file: built-ins are served.
	write("harnesses:\n  - name: aider\n    efforts: [high]\n")
	if _, err := LoadHarnesses(); err == nil || len(HarnessNames()) != len(builtinHarnesses) {
		t.Fatalf("bad kind: %v", err)
	}
	write("harnesses:\n  - name: claude\n    efforts: [high; echo x]\n")
	if _, err := LoadHarnesses(); err == nil || Efforts("claude") == nil {
		t.Fatalf("bad token: %v", err)
	}
}

func TestEffortsWithoutFileUseKinds(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got := Efforts("opencode"); len(got) != 0 {
		t.Fatalf("opencode: %v", got)
	}
	if got := Efforts(""); got != nil {
		t.Fatal("empty name")
	}
}
