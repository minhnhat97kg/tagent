package tmux

import (
	"os"
	"testing"
)

// Test binaries never reach the user's own tmux server: it holds the real notify URL (tests sent
// Telegram messages) and the real agent panes. Every tmux call, including child processes, goes to a
// private socket directory; tests that need tmux start their own server there.
func init() {
	if !testing.Testing() {
		return
	}
	dir, err := os.MkdirTemp("/tmp", "tmux-agent-test-") // short: unix socket paths are limited (~104 bytes on macOS)
	if err != nil {
		panic(err)
	}
	os.Setenv("TMPDIR", "/tmp") // t.TempDir() hosts tmux sockets in some tests; macOS's default TMPDIR is too long for them
	os.Setenv("TMUX", "")
	os.Setenv("TMUX_PANE", "")
	os.Setenv("TMUX_TMPDIR", dir)
}
