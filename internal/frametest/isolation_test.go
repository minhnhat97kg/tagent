package frametest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Ticket 116: a scenario's tmux server must not load the user's ~/.tmux.conf. That file
// runs tagent.tmux, which starts `tagent scrape` inside the test server; the scraper then
// dispatches the real board's tickets there.
func TestScenarioServerDoesNotLoadUserTmuxConfig(t *testing.T) {
	// Not skipIfNoTmux: it only looks in /usr/bin and /bin, so Homebrew tmux always skips.
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on this machine")
	}

	home := t.TempDir()
	marker := filepath.Join(t.TempDir(), "user-config-ran")
	conf := "run-shell 'touch " + marker + "'\n"
	if err := os.WriteFile(filepath.Join(home, ".tmux.conf"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	startScenario(t, scenarioPath("frame-empty"))

	if _, err := os.Stat(marker); err == nil {
		t.Fatal("scenario tmux server loaded the user's ~/.tmux.conf; tagent.tmux would start the real scraper")
	}
}
