package scrape

import (
	"strings"
	"time"

	"tagent/internal/tmux"
)

// When an agent exits or is killed, its window keeps the sidebar pane and so
// never closes: a row of empty claude#N windows. sweepOrphanWindows closes a
// window whose only pane is a sidebar, once it was seen alone in two sweeps
// (a window being set up is spared).

var (
	aloneBefore = map[string]bool{} // window id -> only a sidebar in the last sweep
	lastOrphans time.Time
)

func sweepOrphanWindows() {
	if time.Since(lastOrphans) < 10*time.Second {
		return
	}
	lastOrphans = time.Now()
	out, ok := tmux.RunTmux("list-panes", "-a", "-F", "#{window_id}|#{window_panes}|#{"+tmux.PaneRole+"}")
	if !ok {
		return
	}
	alone := orphanWindows(out)
	for w := range alone {
		if aloneBefore[w] {
			tmux.RunTmux("kill-window", "-t", w)
			delete(alone, w)
		}
	}
	aloneBefore = alone
}

// orphanWindows returns the windows that hold exactly one pane, a sidebar.
func orphanWindows(listPanes string) map[string]bool {
	alone := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(listPanes), "\n") {
		f := strings.SplitN(line, "|", 3)
		if len(f) == 3 && f[1] == "1" && f[2] == "sidebar" {
			alone[f[0]] = true
		}
	}
	return alone
}
