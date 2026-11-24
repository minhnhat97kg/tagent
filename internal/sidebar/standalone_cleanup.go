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

// sweepIdlePanes kills the panes of the legacy tagent-bg session that were idle shells
// in the previous sweep too; it returns the ones idle now.
func sweepIdlePanes(prev, keep []string) tea.Cmd {
	return func() tea.Msg {
		// Only the legacy tagent-bg: project sessions are attachable, their shells are the user's.
		var out string
		sessions, _ := tmux.RunTmux("list-sessions", "-F", "#{session_id}|#{session_name}")
		for _, line := range strings.Split(sessions, "\n") {
			f := strings.Split(strings.TrimSpace(line), "|")
			if len(f) == 2 && f[1] == "tagent-bg" {
				l, _ := tmux.RunTmux("list-panes", "-s", "-t", f[0], "-F", "#{pane_id} #{pane_current_command} #{@pane_keep}")
				out += l + "\n"
			}
		}
		if out == "" {
			return cleanupMsg{}
		}
		self := os.Getenv("TMUX_PANE")
		var idle []string
		for _, line := range strings.Split(out, "\n") {
			id, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
			cmd, kept, _ := strings.Cut(rest, " ")
			candidate, kill := sweepDecision(id, cmd, kept, self, keep, prev)
			if !candidate {
				continue
			}
			if kill {
				tmux.RunTmux("kill-pane", "-t", id)
				continue
			}
			idle = append(idle, id)
		}
		return cleanupMsg{idle: idle}
	}
}

// sweepDecision says whether one pane line of the hidden session is an idle
// shell to watch (candidate), and whether it was already idle in the previous
// sweep so it is due to be killed. Kept terminals (@pane_keep=1) are never
// candidates.
func sweepDecision(id, cmd, kept, self string, keep, prev []string) (candidate, kill bool) {
	if id == "" || id == self || slices.Contains(keep, id) || kept == "1" || !isShellCommand(cmd) {
		return false, false
	}
	return true, slices.Contains(prev, id)
}
