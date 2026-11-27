package workplace

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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

// webRepo makes a repo whose `make web` concatenates web/src into internal/web/dist,
// standing in for the npm build so the test needs no node.
func webRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not installed")
	}
	repo := newRepo(t)
	write(t, repo, "Makefile", ".PHONY: web\nweb:\n\tmkdir -p internal/web/dist && cat web/src/* > internal/web/dist/app.js\n")
	write(t, repo, "web/src/a.txt", "a\n")
	write(t, repo, "web/src/b.txt", "b\n")
	commitBuild(t, repo, "base")
	return repo
}

func write(t *testing.T, dir, rel, s string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// commitBuild runs `make web` in dir, then commits everything, as a ticket does.
func commitBuild(t *testing.T, dir, msg string) {
	t.Helper()
	if out, err := exec.Command("make", "-C", dir, "web").CombinedOutput(); err != nil {
		t.Fatalf("make web: %s", out)
	}
	if _, err := Run(dir, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(dir, "commit", "-qm", msg); err != nil {
		t.Fatal(err)
	}
}

// Two branches that both rebuilt dist merge without help, and dist matches a fresh
// build of the merged web/src.
func TestGateResolvesDistConflictByRebuild(t *testing.T) {
	repo := webRepo(t)
	wt, err := Git{}.Create(repo, "21")
	if err != nil {
		t.Fatal(err)
	}
	write(t, wt, "web/src/b.txt", "b2\n")
	commitBuild(t, wt, "ticket")
	write(t, repo, "web/src/a.txt", "a2\n")
	commitBuild(t, repo, "main")
	if _, err := (Git{}).Gate(repo, "21", "title", nil); err != nil {
		t.Fatalf("Gate: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(repo, webDist, "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "a2\nb2\n"; string(got) != want {
		t.Errorf("dist = %q, want fresh build %q", got, want)
	}
	if out, _ := Run(repo, "status", "--porcelain"); out != "" {
		t.Errorf("main checkout dirty after merge: %q", out)
	}
}

// Bundlers name the output after its content, so two rebuilds rename the same file
// two ways. That conflict is resolved too, leaving only the fresh build.
func TestGateResolvesDistRenameConflict(t *testing.T) {
	repo := webRepo(t)
	write(t, repo, "Makefile", ".PHONY: web\nweb:\n\trm -rf internal/web/dist && mkdir -p internal/web/dist && cat web/src/* > internal/web/dist/app-$$(cat web/src/* | cksum | cut -d' ' -f1).js\n")
	common := strings.Repeat("a line both sides keep, so git sees a rename\n", 30)
	write(t, repo, "web/src/c.txt", common)
	commitBuild(t, repo, "hashed names")
	wt, err := Git{}.Create(repo, "23")
	if err != nil {
		t.Fatal(err)
	}
	write(t, wt, "web/src/b.txt", "b2\n")
	commitBuild(t, wt, "ticket")
	write(t, repo, "web/src/a.txt", "a2\n")
	commitBuild(t, repo, "main")
	if _, err := (Git{}).Gate(repo, "23", "title", nil); err != nil {
		t.Fatalf("Gate: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(repo, webDist, "*"))
	if err != nil || len(files) != 1 {
		t.Fatalf("dist = %v %v, want one bundle", files, err)
	}
	if got, _ := os.ReadFile(files[0]); string(got) != "a2\nb2\n"+common {
		t.Errorf("bundle = %q, want fresh build", got)
	}
	if out, _ := Run(repo, "status", "--porcelain"); out != "" {
		t.Errorf("main checkout dirty after merge: %q", out)
	}
}

// A conflict in web/src is not auto-resolved: the rebase is aborted and main is unchanged.
func TestGateSourceConflictStillBlocks(t *testing.T) {
	repo := webRepo(t)
	wt, err := Git{}.Create(repo, "22")
	if err != nil {
		t.Fatal(err)
	}
	write(t, wt, "web/src/a.txt", "ticket\n")
	commitBuild(t, wt, "ticket")
	write(t, repo, "web/src/a.txt", "main\n")
	commitBuild(t, repo, "main")
	before, _ := Run(repo, "rev-parse", "HEAD")
	if _, err := (Git{}).Gate(repo, "22", "title", nil); err == nil {
		t.Fatal("Gate merged a source conflict")
	}
	if after, _ := Run(repo, "rev-parse", "HEAD"); after != before {
		t.Errorf("main moved: %s -> %s", before, after)
	}
	if _, err := Run(wt, "rev-parse", "--verify", "-q", "REBASE_HEAD"); err == nil {
		t.Error("rebase left in progress")
	}
	if _, err := Run(repo, "rev-parse", "--verify", "-q", "refs/heads/agent/workflow-22"); err != nil {
		t.Error("branch removed")
	}
}
