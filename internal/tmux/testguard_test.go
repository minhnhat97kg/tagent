package tmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoFallbackToDefaultServer(t *testing.T) {
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME"} {
		if !strings.HasPrefix(os.Getenv(key), "/tmp/tagent-test-") {
			t.Fatalf("%s = %q reaches the user's files", key, os.Getenv(key))
		}
	}
	t.Setenv("TMUX_TMPDIR", filepath.Join(t.TempDir(), "gone"))
	if _, err := RunTmuxCapture("list-sessions"); err == nil || !strings.Contains(err.Error(), "not falling back") {
		t.Fatalf("a missing TMUX_TMPDIR must not reach tmux: %v", err)
	}
}
