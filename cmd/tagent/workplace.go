package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"tagent/internal/tasks"
	"tagent/internal/workplace"
)

// cmdWorkplace prunes ticket worktrees. A dry run is the default; --yes removes
// the done (or ticket-less) ones that are clean and merged, never forcing.
func cmdWorkplace(args []string, out, errs io.Writer) int {
	if len(args) == 0 || args[0] != "prune" {
		return exitUsage
	}
	yes := false
	for _, a := range args[1:] {
		switch a {
		case "--yes":
			yes = true
		case "--dry-run":
		default:
			return exitUsage
		}
	}
	wd, _ := os.Getwd()
	root := tasks.MainRoot(wd)
	scope := tasks.Scope(root)
	failed := false
	for _, id := range workplace.Tickets(root) {
		path := filepath.Join(root, workplace.Dir, "workflow-"+id)
		state := "ticket missing"
		t, err := tasks.Get(scope, id)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			fmt.Fprintf(out, "skip %s (ticket: %v)\n", path, err)
			continue
		case t.Status != "done":
			fmt.Fprintf(out, "skip %s (ticket is %s)\n", path, t.Status)
			continue
		default:
			state = "ticket done"
		}
		if why := (workplace.Git{}).PruneBlocker(root, id); why != "" {
			fmt.Fprintf(out, "skip %s (%s)\n", path, why)
			continue
		}
		how := "branch merged"
		if _, err := os.Stat(path); err == nil && !workplace.Usable(path) {
			how = "merged, unusable dir (untracked files not checked)"
		}
		if !yes {
			fmt.Fprintf(out, "remove %s (%s, %s)\n", path, state, how)
			continue
		}
		if notes := (workplace.Git{}).Cleanup(root, id); len(notes) > 0 {
			for _, n := range notes {
				fmt.Fprintln(errs, n)
			}
			failed = true
			continue
		}
		fmt.Fprintf(out, "removed %s (%s, %s)\n", path, state, how)
	}
	if !yes {
		fmt.Fprintln(out, "dry run: nothing removed; pass --yes to remove the ones marked remove")
	}
	if failed {
		return exitFailure
	}
	return exitOK
}
