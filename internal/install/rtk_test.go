package install

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rtkEnv is testEnv with a fake rtk binary and a recorded command list.
func rtkEnv(t *testing.T) (env, *[]string) {
	e := testEnv(t)
	var calls []string
	e.rtk = "/fake/bin/rtk"
	e.run = func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}
	return e, &calls
}

func touch(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRTKWiresEachHarnessWithItsFlags(t *testing.T) {
	e, calls := rtkEnv(t)
	for _, n := range []string{"claude", "codex", "pi", "opencode", "copilot", "gemini", "amp"} {
		e.rtkStep(n, false)
	}
	want := "/fake/bin/rtk init -g --auto-patch|/fake/bin/rtk init -g --codex|/fake/bin/rtk init -g --agent pi --auto-patch|/fake/bin/rtk init -g --opencode --auto-patch|/fake/bin/rtk init -g --copilot --auto-patch"
	if got := strings.Join(*calls, "|"); got != want {
		t.Fatalf("calls = %q", got)
	}
	if _, err := os.Stat(e.marker("rtk-gemini")); err == nil {
		t.Fatal("gemini must never be wired")
	}
}

// Wiring that was already there is left alone: no rtk call, no marker, so
// uninstall never touches it.
func TestRTKKeepsPreexistingWiring(t *testing.T) {
	e, calls := rtkEnv(t)
	touch(t, e.path(".pi/agent/extensions/rtk.ts"), "user's own")
	e.rtkStep("pi", false)
	e.rtkStep("pi", true)
	if len(*calls) != 0 {
		t.Fatalf("rtk ran on pre-existing wiring: %v", *calls)
	}
	if b, _ := os.ReadFile(e.path(".pi/agent/extensions/rtk.ts")); string(b) != "user's own" {
		t.Fatalf("pre-existing file changed: %q", b)
	}
}

// claude's uninstall deletes the opencode plugin too; it must come back.
func TestRTKClaudeUninstallKeepsOpenCodePlugin(t *testing.T) {
	e, calls := rtkEnv(t)
	plugin := e.path(rtkOpenCodePlugin)
	touch(t, plugin, "opencode rtk plugin")
	touch(t, e.marker("rtk-claude"), "")
	e.run = func(name string, args ...string) error {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		os.Remove(plugin) // what rtk does
		return nil
	}
	e.rtkStep("claude", true)
	if want := "/fake/bin/rtk init -g --uninstall"; len(*calls) != 1 || (*calls)[0] != want {
		t.Fatalf("calls = %v", *calls)
	}
	if b, _ := os.ReadFile(plugin); string(b) != "opencode rtk plugin" {
		t.Fatalf("opencode plugin not restored: %q", b)
	}
	if _, err := os.Stat(e.marker("rtk-claude")); err == nil {
		t.Fatal("marker not removed")
	}
}

func TestRTKOpenCodeUninstallRemovesOwnPlugin(t *testing.T) {
	e, calls := rtkEnv(t)
	touch(t, e.path(rtkOpenCodePlugin), "x")
	touch(t, e.marker("rtk-opencode"), "")
	e.rtkStep("opencode", true)
	if len(*calls) != 0 {
		t.Fatalf("opencode uninstall must not run rtk: %v", *calls)
	}
	if _, err := os.Stat(e.path(rtkOpenCodePlugin)); err == nil {
		t.Fatal("plugin still there")
	}
}

// A failing rtk is a warning, not an error, and leaves no marker.
func TestRTKFailureIsWarning(t *testing.T) {
	e, _ := rtkEnv(t)
	e.run = func(string, ...string) error { return os.ErrPermission }
	e.rtkStep("codex", false)
	if _, err := os.Stat(e.marker("rtk-codex")); err == nil {
		t.Fatal("marker written after failure")
	}
	if !strings.Contains(e.out.(*bytes.Buffer).String(), "warn") {
		t.Fatal("no warning printed")
	}
}

func TestRTKBinaryKeptWhileHarnessWired(t *testing.T) {
	e, _ := rtkEnv(t)
	bin := e.path(".local/bin/rtk")
	touch(t, bin, "bin")
	touch(t, e.marker("rtk-bin"), bin+"\n")
	touch(t, e.marker("rtk-claude"), "")
	if err := e.rtkCleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatal("binary removed while claude still wired")
	}
	os.Remove(e.marker("rtk-claude"))
	if err := e.rtkCleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err == nil {
		t.Fatal("binary not removed")
	}
	if _, err := os.Stat(e.marker("rtk-bin")); err == nil {
		t.Fatal("rtk-bin marker not removed")
	}
}

func TestEnsureRTKInstallsOnlyWhenMissing(t *testing.T) {
	e, calls := rtkEnv(t)
	e.rtk = ""
	t.Setenv("PATH", t.TempDir())
	t.Setenv("RTK_INSTALL_DIR", filepath.Join(e.home, "bin"))
	t.Setenv("RTK_EDITION", "")
	t.Setenv("TAGENT_NO_RTK", "")
	e.run = func(name string, args ...string) error {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		touch(t, filepath.Join(e.home, "bin", "rtk"), "")
		return nil
	}
	got := e.ensureRTK()
	if got != filepath.Join(e.home, "bin", "rtk") {
		t.Fatalf("ensureRTK = %q", got)
	}
	if len(*calls) != 1 || !strings.Contains((*calls)[0], "RTK_EDITION=oss") {
		t.Fatalf("calls = %v", *calls)
	}
	if b, _ := os.ReadFile(e.marker("rtk-bin")); !strings.Contains(string(b), "bin/rtk") {
		t.Fatalf("rtk-bin marker = %q", b)
	}

	t.Setenv("TAGENT_NO_RTK", "1")
	if e.ensureRTK() != "" || len(*calls) != 1 {
		t.Fatal("TAGENT_NO_RTK=1 must skip rtk entirely")
	}
}

// Code Mode is a global env var in ~/.claude/settings.json: install adds it,
// a value the user set (0 = opt out) is kept, uninstall removes only what we added.
func TestCodeModeGlobalSetting(t *testing.T) {
	e := testEnv(t)
	cfgPath := e.path(".claude/settings.json")

	if err := e.codeMode(true); err != nil {
		t.Fatal(err)
	}
	if env := readJSON(t, cfgPath)["env"].(map[string]any); env[codeModeVar] != "1" {
		t.Fatalf("env = %v", env)
	}
	if _, err := os.Stat(e.marker("claude-code-mode")); err != nil {
		t.Fatal("marker not written")
	}
	if err := e.codeMode(false); err != nil {
		t.Fatal(err)
	}
	if _, ok := readJSON(t, cfgPath)["env"]; ok {
		t.Fatal("env left behind after uninstall")
	}
	if _, err := os.Stat(e.marker("claude-code-mode")); err == nil {
		t.Fatal("marker not removed")
	}

	touch(t, cfgPath, `{"env":{"CLAUDE_CODE_EXPERIMENTAL_CODE_MODE":"0","OTHER":"x"}}`)
	if err := e.codeMode(true); err != nil {
		t.Fatal(err)
	}
	if env := readJSON(t, cfgPath)["env"].(map[string]any); env[codeModeVar] != "0" {
		t.Fatalf("user opt-out overwritten: %v", env)
	}
	if _, err := os.Stat(e.marker("claude-code-mode")); err == nil {
		t.Fatal("marker written for a value tagent did not set")
	}
	if err := e.codeMode(false); err != nil {
		t.Fatal(err)
	}
	if env := readJSON(t, cfgPath)["env"].(map[string]any); env[codeModeVar] != "0" || env["OTHER"] != "x" {
		t.Fatalf("uninstall touched user env: %v", env)
	}
}
