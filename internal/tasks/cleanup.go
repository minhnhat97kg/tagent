package tasks

import (
	"os"
	"path/filepath"
	"strings"

	"tagent/internal/tmux"
	"tagent/internal/workplace"
)

// cleanupDone tidies a ticket that just reached done: its workflow work is merged
// into the main checkout, then its panes are killed, and its worktree and branch
// are removed when that loses nothing. A failed merge moves the ticket to blocked
// and keeps everything. What was kept, and why, goes to the ticket history. Best
// effort: it never fails Update.
func cleanupDone(scope, id string) {
	root, ok := scopeRoots.Load(scope)
	if !ok {
		return
	}
	r := root.(string)
	var wp workplace.Workplace = workplace.Git{}
	wt := wp.Output(r, id)
	keepOutputs(scope, id, wt)
	note, err := mergeTicket(wp, scope, id, r, wt)
	if err != nil {
		msg := "merge failed: " + err.Error() + "; worktree kept at " + wt
		Comment(scope, id, "tagent", msg, nil)
		Move(scope, id, "blocked", msg)
		return
	}
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

// mergeTicket gates the ticket's checkout into the main checkout. The ticket is
// read first (its title names the commit); if that fails with a checkout present,
// nothing is committed or merged.
func mergeTicket(wp workplace.Workplace, scope, id, root, wt string) (string, error) {
	title := ""
	if _, statErr := os.Stat(wt); statErr == nil {
		t, err := Get(scope, id)
		if err != nil {
			return "", err
		}
		title = t.Title
	}
	return wp.Gate(root, id, title)
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
