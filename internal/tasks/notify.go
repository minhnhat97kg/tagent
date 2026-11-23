package tasks

import (
	"cmp"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"tmux-agent/internal/notify"
)

// Captured under the board lock; delivery happens only after persistence and unlock.
type statusNotice struct {
	scope, id, title, from, to string
	assignees                  []string
}

func newStatusNotice(scope string, t Task, from, to string) *statusNotice {
	return &statusNotice{scope, t.ID, t.Title, from, to, append([]string(nil), t.Assignees...)}
}

// send reports a ticket reaching done or blocked with what a person needs to act on it:
// the outcome (or the question), who said it, and the command to answer.
func (n *statusNotice) send() {
	if n == nil || n.to != "done" && n.to != "blocked" { // only final states reach the phone
		return
	}
	clip := func(s string, max int) string { // stay well below Telegram's 4096 characters
		r := []rune(strings.TrimSpace(s))
		if len(r) > max {
			return string(r[:max]) + "…"
		}
		return string(r)
	}
	repo := n.scope
	if d, ok := scopeRoots.Load(n.scope); ok {
		repo = filepath.Base(d.(string))
	}
	t, _ := Get(n.scope, n.id) // re-read: the comment written with this move is saved now
	var last TaskComment
	for _, c := range t.Comments {
		if c.Author != "system" {
			last = c
		}
	}
	lines := []string{}
	if n.to == "done" {
		head := fmt.Sprintf("✅ %s · #%s done", repo, n.id)
		if !t.Created.IsZero() {
			head += " in " + time.Since(t.Created).Round(time.Minute).String()
		}
		lines = append(lines, head, clip(n.title, 200))
		if summary := cmp.Or(t.Note, last.Body); summary != "" {
			lines = append(lines, "", clip(summary, 1500))
		}
	} else {
		lines = append(lines, fmt.Sprintf("⛔ %s · #%s blocked at %s", repo, n.id, n.from), clip(n.title, 200))
		cfg, _ := ConfigForScopeTask(n.scope, t)
		history, _ := ReadEvents(n.scope, 0)
		_, role, reason := BlockContext(t, cfg, history)
		if reason == "" {
			reason = last.Body
		}
		if reason != "" {
			lines = append(lines, "", cmp.Or(role, last.Author, "agent")+": "+clip(reason, 1500))
		}
		lines = append(lines, "", fmt.Sprintf("Answer: tmux-agent task comment %s \"@%s …\"", n.id, cmp.Or(role, "<role>")))
	}
	if len(n.assignees) > 0 {
		lines = append(lines, "Assignees: "+clip(strings.Join(n.assignees, ", "), 200))
	}
	notify.Send("Ticket "+n.to, strings.Join(lines, "\n"))
}
