package tasks

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"tagent/internal/tmux"
	"tagent/internal/workplace"
)

// gateDone merges the ticket's checkout into the main checkout before done is
// recorded. A refused merge comes back as a tagent comment for the caller to save
// on a blocked ticket. It runs under the board lock, so it must not call Comment,
// Move, Update or DispatchAudit, which take the lock again.
// ponytail: git runs under the board lock; per-repo merge lock if it ever stalls board writes.
func gateDone(scope string, t *Task) (note string, refused *TaskComment, err error) {
	root, ok := scopeRoots.Load(scope)
	if !ok {
		return "", nil, nil
	}
	r := root.(string)
	wp := ticketWorkplace(scope, *t)
	note, gateErr := wp.Gate(r, t.ID, t.Title)
	if gateErr == nil {
		return note, nil, nil
	}
	c := TaskComment{ID: strconv.FormatInt(time.Now().UnixNano(), 36), Author: "tagent", Body: "merge failed: " + gateErr.Error() + "; worktree kept at " + wp.Output(r, t.ID), Created: time.Now().UTC(), Attachments: []Attachment{}}
	if err := compactBody(scope, t.ID, &c); err != nil {
		return "", nil, err
	}
	t.Comments = append(t.Comments, c)
	return "", &c, nil
}

// cleanupDone tidies a ticket that has just reached done, after gateDone merged it:
// its outputs are kept, the merge note (if any) goes to the history, then its
// worktree and branch are removed when that loses nothing. Its panes are killed
// last, after the removal, so no pane is closed mid-removal. A kept worktree keeps
// its panes: its agents may still be working in it. What was kept, and why, goes
// to the ticket history. Best effort.
func cleanupDone(scope, id, note string) {
	root, ok := scopeRoots.Load(scope)
	if !ok {
		return
	}
	r := root.(string)
	t, err := Get(scope, id)
	if err != nil {
		return
	}
	panes := ticketPanes(scope, id) // before removal: Scope() of a removed worktree no longer resolves
	wp := ticketWorkplace(scope, t)
	keepOutputs(scope, id, wp.Output(r, id))
	if note != "" {
		Comment(scope, id, "tagent", note, nil)
	}
	notes := wp.Cleanup(r, id)
	for _, n := range notes {
		DispatchAudit(scope, id, n)
		if strings.HasPrefix(n, "cleanup: kept worktree") { // its agents may still be working in it
			return
		}
	}
	killPanes(panes)
}

// ticketWorkplace is the workplace the ticket ran in. A lookup error keeps the
// default git worktree, which is a no-op when none exists.
func ticketWorkplace(scope string, t Task) workplace.Workplace {
	if wp, err := TicketWorkplace(scope, t); err == nil {
		return wp
	}
	return workplace.Git{}
}

// keepOutputs copies the ticket's output files that live in its worktree (often
// git-ignored, e.g. .tasks/plans/ID.md) into the ticket's attachments, so
// removing the worktree does not lose them.
func keepOutputs(scope, id, wt string) {
	t, err := Get(scope, id)
	if err != nil {
		return
	}
	var files []string
	for _, values := range t.Outputs {
		for _, v := range values {
			if rel, e := filepath.Rel(wt, v); filepath.IsAbs(v) && e == nil {
				v = rel
			}
			if !filepath.IsLocal(v) {
				continue
			}
			if st, e := os.Lstat(filepath.Join(wt, v)); e == nil && st.Mode().IsRegular() {
				files = append(files, filepath.Join(wt, v))
			}
		}
	}
	if len(files) > 0 {
		Comment(scope, id, "tagent", "cleanup: kept outputs of the worktree", files)
	}
}

// ticketPanes lists the panes of a ticket: its chat panes (@pane_ticket) and the
// role panes dispatched for it in its repo (@board_target "role#id").
func ticketPanes(scope, id string) []string {
	var ids []string
	if panes, err := tmux.TicketPanes(); err == nil {
		for _, p := range panes {
			if p.Ticket == scope+"#"+id {
				ids = append(ids, p.PaneID)
			}
		}
	}
	if panes, err := tmux.BoardPanes(); err == nil {
		for _, p := range panes {
			if _, ticket, ok := strings.Cut(p.BoardTarget, "#"); ok && ticket == id && Scope(p.Path) == scope && !slices.Contains(ids, p.PaneID) {
				ids = append(ids, p.PaneID)
			}
		}
	}
	return ids
}

func killPanes(ids []string) {
	self := os.Getenv("TMUX_PANE")
	for _, id := range ids {
		if id != self {
			tmux.RunTmux("kill-pane", "-t", id)
		}
	}
	if self != "" && slices.Contains(ids, self) { // the caller may be one of them: last, it ends this process
		tmux.RunTmux("kill-pane", "-t", self)
	}
}
