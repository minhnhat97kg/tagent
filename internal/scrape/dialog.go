package scrape

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"tagent/internal/hook"
	"tagent/internal/tmux"
)

// Dialog is an approval/question prompt read off an agent's screen.
type Dialog struct {
	Question string
	Options  []Option
	// Arrow: an unnumbered menu picked with Up/Down + Enter (Claude's
	// "trust this folder"); Selected is the highlighted option. Options
	// still get keys 1..n so the sidebar answers them the same way.
	Arrow    bool
	Selected int
	// Confirm: a digit only highlights the option; Enter submits it
	// (Codex: "enter continue", "Press enter to confirm").
	Confirm bool
}

// Option is one answer: the key that picks it, and its text.
type Option struct {
	Key, Label string
}

var (
	optionRe  = regexp.MustCompile(`^\s*(?:[❯›>▶]\s*)?(\d)[.)]\s+(.+?)\s*$`)
	yesNoRe   = regexp.MustCompile(`(?i)[\[(]y/n[\])]`)
	boxChars  = "│┃|╎╏┆┊ \t"
	confirmRe = regexp.MustCompile(`(?i)(^|\s)(press )?enter (to )?(continue|confirm)`)
	arrowRe   = regexp.MustCompile(`^(\s*[❯›>▶]\s+)(\S.*?)\s*$`)
)

// ParseDialog finds the last prompt on screen (the blockedRe line Classify
// uses) and the numbered options under it — Claude Code's "❯ 1. Yes /
// 2. Yes, and don't ask again… / 3. No" — or a [y/n] question. ok=false
// when there's no prompt or no recognizable options.
func ParseDialog(screen string) (Dialog, bool) {
	// A capture is padded with blank lines to the pane height; drop them so
	// "the last lines" means the end of the content, not of the pane.
	lines := strings.Split(strings.TrimRight(screen, "\n \t"), "\n")
	q := -1
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-tailLines-10; i-- {
		if blockedRe.MatchString(lines[i]) && !optionRe.MatchString(lines[i]) && !strings.Contains(strings.ToLower(lines[i]), "enter to confirm") {
			q = i
			break
		}
	}
	if q < 0 {
		return Dialog{}, false
	}
	d := Dialog{Question: strings.Trim(lines[q], boxChars)}
	if yesNoRe.MatchString(lines[q]) {
		d.Options = []Option{{"y", "Yes"}, {"n", "No"}}
		return d, true
	}
	for _, l := range lines[q+1:] {
		if m := optionRe.FindStringSubmatch(l); m != nil {
			d.Options = append(d.Options, Option{Key: m[1], Label: m[2]})
		}
	}
	if len(d.Options) > 0 {
		for _, l := range lines[q+1:] {
			d.Confirm = d.Confirm || confirmRe.MatchString(l)
		}
		return d, true
	}
	labels, sel, top, bottom, ok := arrowMenu(lines)
	if !ok {
		return Dialog{}, false
	}
	if q >= top && q <= bottom { // the prompt matched on an option's own text
		d.Question = "Choose an option"
	}
	for i, l := range labels {
		d.Options = append(d.Options, Option{Key: strconv.Itoa(i + 1), Label: l})
	}
	d.Arrow, d.Selected = true, sel
	return d, true
}

// arrowMenu finds the last unnumbered menu: a highlighted "❯ text" line and
// the lines around it whose text starts in the same column. Needs at least
// two options.
func arrowMenu(lines []string) (labels []string, sel, top, bottom int, ok bool) {
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-tailLines-10; i-- {
		m := arrowRe.FindStringSubmatch(lines[i])
		if m == nil || optionRe.MatchString(lines[i]) {
			continue
		}
		col := len([]rune(m[1]))
		at := func(j int) bool {
			l := lines[j]
			text := strings.TrimLeft(l, " ")
			return text != "" && len(l)-len(text) == col
		}
		top, bottom = i, i
		for top > 0 && at(top-1) {
			top--
		}
		for bottom+1 < len(lines) && at(bottom+1) {
			bottom++
		}
		if bottom == top {
			return nil, 0, 0, 0, false
		}
		for j := top; j <= bottom; j++ {
			if j == i {
				labels = append(labels, m[2])
			} else {
				labels = append(labels, strings.TrimSpace(lines[j]))
			}
		}
		return labels, i - top, top, bottom, true
	}
	return nil, 0, 0, 0, false
}

// arrowKeys moves the highlight from d.Selected to option key, then Enter.
func arrowKeys(d Dialog, key string) []string {
	n, err := strconv.Atoi(key)
	if err != nil || n < 1 || n > len(d.Options) {
		return nil
	}
	var keys []string
	for i := d.Selected; i < n-1; i++ {
		keys = append(keys, "Down")
	}
	for i := d.Selected; i > n-1; i-- {
		keys = append(keys, "Up")
	}
	return append(keys, "Enter")
}

// AnswerKeys is what to type to pick option key: a digit selects in
// numbered menus; y/n questions want the letter then Enter.
func AnswerKeys(key string) []string {
	if key == "y" || key == "n" {
		return []string{key, "Enter"}
	}
	return []string{key}
}

// Answer types the answer for option key into pane, then moves the pane out
// of `waiting` as soon as the dialog is gone — no hook reports an answer,
// and waiting for the scraper's ticks leaves "needs approval" up for
// seconds. Running shows at once; idle must hold for idleHold, since the
// screen is briefly idle between the dialog closing and the spinner. If
// neither settles within answerWait, the scraper takes over.
func Answer(pane, key string) {
	keys := AnswerKeys(key)
	// An arrow menu has no digit shortcuts: re-read it and navigate instead.
	if screen, ok := tmux.RunTmux("capture-pane", "-p", "-t", pane); ok {
		if d, ok := ParseDialog(screen); ok && d.Arrow {
			if keys = arrowKeys(d, key); keys == nil {
				return
			}
		} else if ok && d.Confirm && key != "y" && key != "n" {
			keys = append(keys, "Enter")
		}
	}
	tmux.RunTmux(append([]string{"send-keys", "-t", pane}, keys...)...)
	const (
		poll       = 100 * time.Millisecond
		idleHold   = 5 // polls
		answerWait = 20
	)
	idle := 0
	for i := 0; i < answerWait; i++ {
		time.Sleep(poll)
		screen, ok := tmux.RunTmux("capture-pane", "-p", "-t", pane)
		if !ok {
			return
		}
		next := Classify(screen)
		if next == tmux.StatusIdle {
			idle++
		} else {
			idle = 0
		}
		if next == tmux.StatusRunning || idle >= idleHold {
			if tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt) == string(tmux.StatusWaiting) {
				hook.Transition(pane, tmux.GetPaneOptionValue(pane, tmux.PaneAgent), tmux.StatusWaiting, next)
			}
			return
		}
	}
}
