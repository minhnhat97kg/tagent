package tmux

import (
	"os"
	"path/filepath"
	"testing"
)

// Test binaries never reach the user's own tmux server: it holds the real notify URL (tests sent
// remote push messages) and the real agent panes. Every tmux call, including child processes, goes to a
// private socket directory; tests that need tmux start their own server there.
func init() {
	if !testing.Testing() {
		return
	}
	dir, err := os.MkdirTemp("/tmp", "tagent-test-") // short: unix socket paths are limited (~104 bytes on macOS)
	if err != nil {
		panic(err)
	}
	os.Setenv("TMPDIR", "/tmp") // t.TempDir() hosts tmux sockets in some tests; macOS's default TMPDIR is too long for them
	os.Setenv("TMUX", "")
	os.Setenv("TMUX_PANE", "")
	os.Setenv("TMUX_TMPDIR", dir)
	// Nor the user's settings and state: with their Telegram token, a hook run by
	// a test starts a detached sender.
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
}
