package workplace

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// newRepo makes a repo with one commit, and returns its path.
func newRepo(t *testing.T) string {
	t.Helper()
	repo, _ := filepath.EvalSymlinks(t.TempDir())
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@test")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@test")
	for _, args := range [][]string{{"init", "-q"}, {"commit", "--allow-empty", "-qm", "base"}} {
		if _, err := Run(repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

// commitIn makes a commit in dir, which is then the checkout's HEAD.
func commitIn(t *testing.T, dir, msg string) {
	t.Helper()
	if _, err := Run(dir, "commit", "--allow-empty", "-qm", msg); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestCleanupAndPrune(t *testing.T) {
	t.Run("merged and clean is removed with its branch", func(t *testing.T) {
		repo := newRepo(t)
		wt, err := Git{}.Create(repo, "1")
		if err != nil {
			t.Fatal(err)
		}
		commitIn(t, wt, "work")
		if _, err := (Git{}).Gate(repo, "1", "title"); err != nil {
			t.Fatal(err)
		}
		if notes := (Git{}).Cleanup(repo, "1"); len(notes) != 0 {
			t.Fatalf("notes = %v", notes)
		}
		if exists(wt) {
			t.Error("worktree left behind")
		}
		if _, err := Run(repo, "rev-parse", "--verify", "-q", "refs/heads/agent/workflow-1"); err == nil {
			t.Error("branch left behind")
		}
	})

	t.Run("detached HEAD with a commit is kept", func(t *testing.T) {
		repo := newRepo(t)
		wt, _ := Git{}.Create(repo, "2")
		if _, err := Run(wt, "checkout", "-q", "--detach"); err != nil {
			t.Fatal(err)
		}
		commitIn(t, wt, "detached")
		if _, err := (Git{}).Gate(repo, "2", "title"); err != nil {
			t.Fatal(err)
		}
		if why := (Git{}).PruneBlocker(repo, "2"); why == "" {
			t.Error("prune would remove a detached checkout")
		}
		if notes := (Git{}).Cleanup(repo, "2"); len(notes) != 1 {
			t.Errorf("notes = %v, want one kept note", notes)
		}
		if !exists(wt) {
			t.Error("detached worktree was removed; its commit is now unreachable")
		}
	})

	t.Run("keep leaves worktree and branch", func(t *testing.T) {
		repo := newRepo(t)
		wt, _ := Git{Keep: true}.Create(repo, "3")
		commitIn(t, wt, "work")
		if _, err := (Git{Keep: true}).Gate(repo, "3", "title"); err != nil {
			t.Fatal(err)
		}
		if notes := (Git{Keep: true}).Cleanup(repo, "3"); len(notes) != 0 {
			t.Fatalf("notes = %v", notes)
		}
		if !exists(wt) {
			t.Error("keep removed the worktree")
		}
	})

	t.Run("unmerged worktree and branch-only ids are judged by merge state", func(t *testing.T) {
		repo := newRepo(t)
		wt, _ := Git{}.Create(repo, "4") // unmerged: commit only on its branch
		commitIn(t, wt, "unmerged")
		if why := (Git{}).PruneBlocker(repo, "4"); why != "not merged" {
			t.Errorf("unmerged worktree: blocker = %q", why)
		}
		if _, err := Run(repo, "branch", "agent/workflow-5"); err != nil { // merged, no worktree
			t.Fatal(err)
		}
		if why := (Git{}).PruneBlocker(repo, "5"); why != "" {
			t.Errorf("merged branch only: blocker = %q", why)
		}
		if got := Tickets(repo); !slices.Equal(got, []string{"4", "5"}) {
			t.Errorf("Tickets = %v", got)
		}
		(Git{}).Cleanup(repo, "5")
		if _, err := Run(repo, "rev-parse", "--verify", "-q", "refs/heads/agent/workflow-5"); err == nil {
			t.Error("merged branch-only ticket was not removed")
		}
		if !exists(wt) {
			t.Error("unmerged worktree removed")
		}
	})
}
