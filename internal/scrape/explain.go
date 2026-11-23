package scrape

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"tagent/internal/process"
	"tagent/internal/tmux"
)

// Explain says why a pane has the status it has: where the status came
// from, which screen rule matches, and what its process tree is doing.
// Like `herdr agent explain`, for when a status looks wrong.
func Explain(pane string) string {
	var b strings.Builder
	opt := func(name string) string { return tmux.GetPaneOptionValue(pane, name) }
	src := opt(tmux.PaneAgentSrc)
	if src == "" {
		src = "hook"
	}
	fmt.Fprintf(&b, "pane      %s\nagent     %s (status from %s)\nstatus    %s", pane, opt(tmux.PaneAgent), src, opt(tmux.PaneStatusOpt))
	if r := opt(tmux.PaneWaitReason); r != "" {
		fmt.Fprintf(&b, " · wait reason %s", r)
	}
	if s := opt(tmux.PaneStuck); s != "" {
		fmt.Fprintf(&b, " · flagged quiet for %sm", s)
	}
	b.WriteString("\n")

	screen, ok := tmux.RunTmux("capture-pane", "-p", "-t", pane)
	if !ok {
		return b.String() + "screen    unreadable\n"
	}
	rule, line := matchedRule(screen)
	fmt.Fprintf(&b, "screen    %s", Classify(screen))
	if line != "" {
		fmt.Fprintf(&b, " — %s: %q", rule, strings.TrimSpace(line))
	} else {
		b.WriteString(" — no waiting/working marker in the bottom lines (idle fallback)")
	}
	if IsInterrupted(screen) {
		b.WriteString(" · shows \"interrupted\"")
	}
	b.WriteString("\n")

	pid, _ := strconv.Atoi(tmux.DisplayMessage(pane, "#{pane_pid}"))
	if snap, ok := process.Scan(); ok && pid > 0 {
		before := process.CPUTimes()
		time.Sleep(time.Second)
		after := process.CPUTimes()
		var burn float64
		n := 0
		for p := range snap.Descendants([]int{pid}) {
			n++
			if d := after[p] - before[p]; d > 0 {
				burn += d
			}
		}
		fmt.Fprintf(&b, "process   %d processes in the tree · %.2fs CPU in the last second · shell tool running: %v\n", n, burn, snap.HasToolChild([]int{pid}))
		b.WriteString("verdict   ")
		switch {
		case burn >= busyThreshold/5:
			b.WriteString("busy — CPU is being used, not stuck\n")
		case snap.HasToolChild([]int{pid}):
			fmt.Fprintf(&b, "quiet but a tool is running — flagged only after %dx the stuck limit\n", toolPatience)
		default:
			fmt.Fprintf(&b, "quiet — flagged after %s without activity\n", stuckAfter())
		}
	}
	return b.String()
}

// matchedRule returns the rule (and screen line) behind Classify's answer.
func matchedRule(screen string) (string, string) {
	lines := strings.Split(strings.TrimRight(screen, "\n "), "\n")
	var tail []string
	for i := len(lines) - 1; i >= 0 && len(tail) < tailLines; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			tail = append(tail, lines[i])
		}
	}
	for _, l := range tail {
		if blockedRe.MatchString(l) {
			return "approval prompt", l
		}
	}
	for i, l := range tail {
		if i < workingLines && workingRe.MatchString(l) {
			return "working indicator", l
		}
	}
	return "", ""
}
