package tasks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"tagent/internal/tmux"
)

// cleanupDone tidies a ticket that just reached done: its panes are killed, and
// its workflow worktree and branch are removed when that loses nothing. What was
// kept, and why, goes to the ticket history. Best effort: it never fails Update.
func cleanupDone(scope, id string) {
	root, ok := scopeRoots.Load(scope)
	if !ok {
		return
	}
	keepOutputs(scope, id, filepath.Join(root.(string), ".worktrees", "workflow-"+id))
	notes := RemoveWorkflowWorktree(root.(string), "workflow-"+id)
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

// RemoveWorkflowWorktree removes .worktrees/<name> only if it has no uncommitted
// changes, and its agent/<name> branch only if merged into the main checkout's
// HEAD. It returns one note per thing it left in place.
func RemoveWorkflowWorktree(root, name string) (notes []string) {
	path := filepath.Join(root, ".worktrees", name)
	branch := "agent/" + name
	if _, err := os.Stat(path); err == nil {
		out, serr := gitOut(path, "status", "--porcelain")
		if serr != nil || out != "" {
			return []string{"cleanup: kept worktree " + path + " (uncommitted changes)"}
		}
		if _, err := gitOut(root, "worktree", "remove", path); err != nil {
			return []string{"cleanup: kept worktree " + path + " (" + err.Error() + ")"}
		}
	}
	if _, err := gitOut(root, "rev-parse", "--verify", "-q", "refs/heads/"+branch); err != nil {
		return nil
	}
	if _, err := gitOut(root, "merge-base", "--is-ancestor", branch, "HEAD"); err != nil {
		return append(notes, "cleanup: kept branch "+branch+" (not merged)")
	}
	if _, err := gitOut(root, "branch", "-d", branch); err != nil {
		notes = append(notes, "cleanup: kept branch "+branch+" ("+err.Error()+")")
	}
	return notes
}

func gitOut(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, &gitError{strings.Join(args, " "), s}
	}
	return s, nil
}

type gitError struct{ cmd, out string }

func (e *gitError) Error() string { return "git " + e.cmd + ": " + e.out }
