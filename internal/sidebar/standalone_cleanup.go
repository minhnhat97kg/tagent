package sidebar

import (
	"os"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tagent/internal/tmux"
)

// A harness that exits leaves its pane behind as an idle shell in the hidden
// session. Standalone mode sweeps those: a pane that is only a shell (no
// harness running in it) and is not the one on screen is killed. A pane must
// look idle in two sweeps in a row, so one that is still starting is spared.

const cleanupEvery = 15 * time.Second

type cleanupTickMsg struct{}
type cleanupMsg struct{ idle []string }

func cleanupTick() tea.Cmd {
	return tea.Tick(cleanupEvery, func(time.Time) tea.Msg { return cleanupTickMsg{} })
}

func isShellCommand(cmd string) bool {
	return slices.Contains([]string{"bash", "zsh", "sh", "fish", "dash", "ksh", "-bash", "-zsh", "-sh", "-fish"}, cmd)
}

// sweepIdlePanes kills the panes of the hidden session that were idle shells
// in the previous sweep too; it returns the ones idle now.
func sweepIdlePanes(prev, keep []string) tea.Cmd {
	return func() tea.Msg {
		out, ok := tmux.RunTmux("list-panes", "-s", "-t", "="+HiddenSession, "-F", "#{pane_id} #{pane_current_command}")
		if !ok {
			return cleanupMsg{}
		}
		self := os.Getenv("TMUX_PANE")
		var idle []string
		for _, line := range strings.Split(out, "\n") {
			id, cmd, _ := strings.Cut(strings.TrimSpace(line), " ")
			if id == "" || id == self || slices.Contains(keep, id) || !isShellCommand(cmd) {
				continue
			}
			if slices.Contains(prev, id) {
				tmux.RunTmux("kill-pane", "-t", id)
				continue
			}
			idle = append(idle, id)
		}
		return cleanupMsg{idle: idle}
	}
}
