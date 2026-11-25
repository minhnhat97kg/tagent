package tasks

import (
	"os"
	"path/filepath"
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
	var wp workplace.Workplace = workplace.Git{}
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
// worktree and branch are removed when that loses nothing and its panes are
// killed. What was kept, and why, goes to the ticket history. Best effort.
func cleanupDone(scope, id, note string) {
	root, ok := scopeRoots.Load(scope)
	if !ok {
		return
	}
	r := root.(string)
	var wp workplace.Workplace = workplace.Git{}
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
	killTicketPanes(scope + "#" + id)
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
		Comment(scope, id, "tagent", "cleanup: kept outputs of the removed worktree", files)
	}
}

func killTicketPanes(ticket string) {
	panes, err := tmux.TicketPanes()
	if err != nil {
		return
	}
	self := os.Getenv("TMUX_PANE")
	for _, p := range panes {
		if p.Ticket == ticket && p.PaneID != self {
			tmux.RunTmux("kill-pane", "-t", p.PaneID)
		}
	}
	if self != "" { // the caller may be one of them: last, it ends this process
		for _, p := range panes {
			if p.Ticket == ticket && p.PaneID == self {
				tmux.RunTmux("kill-pane", "-t", self)
			}
		}
	}
}
