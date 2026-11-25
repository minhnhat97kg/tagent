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
		if _, err := (Git{}).Gate(repo, "1", "title", nil); err != nil {
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
		if _, err := (Git{}).Gate(repo, "2", "title", nil); err != nil {
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
		if _, err := (Git{Keep: true}).Gate(repo, "3", "title", nil); err != nil {
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

// A new ticket reaching done leaves no worktree dir and no branch when its
// checkout was half-removed after the merge: tracked files are gone from disk,
// which git's non-forcing removal refused to finish.
func TestDoneCleanupLeavesNoHalfRemovedWorktree(t *testing.T) {
	repo := newRepo(t)
	wt, err := Git{}.Create(repo, "10")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "f.txt"), []byte("f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(wt, "add", "f.txt"); err != nil {
		t.Fatal(err)
	}
	commitIn(t, wt, "work")
	if _, err := (Git{}).Gate(repo, "10", "title", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(wt, "f.txt")); err != nil { // interrupted removal
		t.Fatal(err)
	}
	if notes := (Git{}).Cleanup(repo, "10"); len(notes) != 0 {
		t.Errorf("notes = %v", notes)
	}
	if exists(wt) {
		t.Error("worktree dir left behind")
	}
	if _, err := Run(repo, "rev-parse", "--verify", "-q", "refs/heads/agent/workflow-10"); err == nil {
		t.Error("branch left behind")
	}
}

// A checkout that is no longer a git worktree (its .git left dangling) is removed
// with its directory and branch once the branch is merged.
func TestCleanupRemovesHalfRemovedCheckout(t *testing.T) {
	repo := newRepo(t)
	wt, _ := Git{}.Create(repo, "11")
	commitIn(t, wt, "work")
	if _, err := (Git{}).Gate(repo, "11", "title", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "left.txt"), []byte("left\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /nowhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if why := (Git{}).PruneBlocker(repo, "11"); why != "" {
		t.Errorf("blocker = %q", why)
	}
	if notes := (Git{}).Cleanup(repo, "11"); len(notes) != 0 {
		t.Errorf("notes = %v", notes)
	}
	if exists(wt) {
		t.Error("half-removed dir left behind")
	}
	if _, err := Run(repo, "rev-parse", "--verify", "-q", "refs/heads/agent/workflow-11"); err == nil {
		t.Error("branch left behind")
	}
}

// Without its own .git a checkout is never gated, since git would commit into the
// main checkout. Its unmerged branch and directory stay.
func TestHalfRemovedUnmergedCheckoutIsKept(t *testing.T) {
	repo := newRepo(t)
	wt, _ := Git{}.Create(repo, "12")
	commitIn(t, wt, "unmerged")
	if err := os.Remove(filepath.Join(wt, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Git{}).Gate(repo, "12", "title", nil); err == nil {
		t.Error("Gate accepted a checkout without its own .git")
	}
	if out, _ := Run(repo, "status", "--porcelain"); out != "?? dirty.txt" {
		t.Errorf("main checkout changed by Gate: %q", out)
	}
	if why := (Git{}).PruneBlocker(repo, "12"); why != "not merged" {
		t.Errorf("blocker = %q", why)
	}
	if notes := (Git{}).Cleanup(repo, "12"); len(notes) != 1 {
		t.Errorf("notes = %v, want one kept note", notes)
	}
	if !exists(wt) {
		t.Error("unmerged half-removed dir removed")
	}
	if _, err := Run(repo, "rev-parse", "--verify", "-q", "refs/heads/agent/workflow-12"); err != nil {
		t.Error("unmerged branch removed")
	}
}

// A half-removed checkout whose branch is already merged has nothing to gate, so
// the ticket can still reach done and Cleanup removes the directory.
func TestGateAllowsMergedHalfRemovedCheckout(t *testing.T) {
	repo := newRepo(t)
	wt, _ := Git{}.Create(repo, "13")
	commitIn(t, wt, "work")
	if _, err := (Git{}).Gate(repo, "13", "title", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(wt, ".git")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Git{}).Gate(repo, "13", "title", nil); err != nil {
		t.Errorf("Gate refused a merged half-removed checkout: %v", err)
	}
	if notes := (Git{}).Cleanup(repo, "13"); len(notes) != 0 {
		t.Errorf("notes = %v", notes)
	}
	if exists(wt) {
		t.Error("half-removed dir left behind")
	}
}

// Discard removes unmerged work, and a worktree whose directory is already gone.
func TestDiscardRemovesUnmergedAndStaleCheckout(t *testing.T) {
	repo := newRepo(t)
	wt, err := Git{}.Create(repo, "2")
	if err != nil {
		t.Fatal(err)
	}
	commitIn(t, wt, "unmerged")
	if err := (Git{}).Discard(repo, "2"); err != nil {
		t.Fatal(err)
	}
	if exists(wt) {
		t.Error("worktree left behind")
	}
	if _, err := Run(repo, "rev-parse", "--verify", "-q", "refs/heads/agent/workflow-2"); err == nil {
		t.Error("unmerged branch left behind")
	}
	wt, err = Git{}.Create(repo, "3")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(wt); err != nil { // stale: directory gone, worktree still registered
		t.Fatal(err)
	}
	if err := (Git{}).Discard(repo, "3"); err != nil {
		t.Fatalf("stale checkout: %v", err)
	}
	if _, err := Run(repo, "rev-parse", "--verify", "-q", "refs/heads/agent/workflow-3"); err == nil {
		t.Error("branch of stale checkout left behind")
	}
}
