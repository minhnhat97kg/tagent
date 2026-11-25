package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The bench's private tmux server must not read the user's ~/.tmux.conf: that file runs hooks and
// plugins that can disturb the pane under test. This goes through startBenchAddr itself, so
// reverting its call to benchTmux makes this fail.
func TestBenchTmuxServerIgnoresUserConfig(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux")
	}
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".tmux.conf"), []byte("set -g @tagent_leak 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	startBenchAddr(t, "cfgprobe", "echo READY; exec cat")
	out, err := exec.Command("tmux", "show", "-gv", "@tagent_leak").CombinedOutput()
	if v := strings.TrimSpace(string(out)); err == nil && v != "" {
		t.Fatalf("private tmux server loaded user config: @tagent_leak=%q", v)
	}
}

// skipUnlessFlaky quarantines load-sensitive tests until #115 fixes them.
func skipUnlessFlaky(t *testing.T) {
	t.Helper()
	if os.Getenv("TAGENT_FLAKY_TESTS") != "1" {
		t.Skip("flaky under load, see #115")
	}
}
