package sidebar

import "testing"

func TestIsShellCommand(t *testing.T) {
	for cmd, want := range map[string]bool{"zsh": true, "-bash": true, "fish": true, "claude": false, "node": false, "pi": false, "lazygit": false, "2.1.330": false} {
		if isShellCommand(cmd) != want {
			t.Errorf("%q: want %v", cmd, want)
		}
	}
}
