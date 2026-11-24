package tasks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupKeepsOutputsAsAttachments(t *testing.T) {
	root := t.TempDir()
	scope := Scope(root)
	task, err := Add(scope, "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(root, ".worktrees", "workflow-"+task.ID, ".tasks", "plans")
	os.MkdirAll(plan, 0o755)
	os.WriteFile(filepath.Join(plan, task.ID+".md"), []byte("plan"), 0o644)
	task.Outputs = map[string]map[string]string{"plan": {"plan": ".tasks/plans/" + task.ID + ".md"}}
	if err := write(scope, task); err != nil {
		t.Fatal(err)
	}
	keepOutputs(scope, task.ID, filepath.Join(root, ".worktrees", "workflow-"+task.ID))
	os.RemoveAll(filepath.Join(root, ".worktrees"))
	list, err := AttachmentList(scope, root, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range list {
		if a.Source == "comment" && a.Name == task.ID+".md" && a.Error == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("plan not attached: %+v", list)
	}
}

// gitRun runs git in dir for the merge tests; the repo is a real one in t.TempDir().
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newMergeRepo returns a git repo with one commit and a ticket whose workflow
// worktree sits at .worktrees/workflow-<id> on agent/workflow-<id>.
func newMergeRepo(t *testing.T) (repo, scope, id, wt string) {
	t.Helper()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@test")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@test")
	repo = t.TempDir()
	gitRun(t, repo, "init", "-q")
	os.WriteFile(filepath.Join(repo, "base.txt"), []byte("base\n"), 0o644)
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-qm", "base")
	scope = Scope(repo)
	task, err := Add(scope, "Fix the thing", nil)
	if err != nil {
		t.Fatal(err)
	}
	id = task.ID
	wt = filepath.Join(repo, ".worktrees", "workflow-"+id)
	gitRun(t, repo, "worktree", "add", "-q", "-b", "agent/workflow-"+id, wt)
	return repo, scope, id, wt
}

// Bug: a dirty workflow worktree is kept on done and its work never reaches main.
func TestDoneMergesDirtyWorktree(t *testing.T) {
	repo, scope, id, wt := newMergeRepo(t)
	os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("fix\n"), 0o644)
	if err := Move(scope, id, "done", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(repo, "fix.txt")); err != nil {
		t.Fatalf("work not merged into main: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatal("worktree kept after merge")
	}
	if b := gitRun(t, repo, "branch", "--list", "agent/workflow-"+id); b != "" {
		t.Fatalf("branch kept after merge: %q", b)
	}
}

func TestDoneRebasesAndMergesWhenMainMovedAhead(t *testing.T) {
	repo, scope, id, wt := newMergeRepo(t)
	os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("fix\n"), 0o644)
	gitRun(t, wt, "add", "-A")
	gitRun(t, wt, "commit", "-qm", "fix")
	os.WriteFile(filepath.Join(repo, "other.txt"), []byte("other\n"), 0o644)
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-qm", "other")
	if err := Move(scope, id, "done", ""); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"fix.txt", "other.txt"} {
		if _, err := os.ReadFile(filepath.Join(repo, f)); err != nil {
			t.Fatalf("%s missing on main: %v", f, err)
		}
	}
}

func TestDoneConflictBlocksAndKeepsWorktree(t *testing.T) {
	repo, scope, id, wt := newMergeRepo(t)
	os.WriteFile(filepath.Join(wt, "base.txt"), []byte("worktree\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "base.txt"), []byte("main\n"), 0o644)
	gitRun(t, repo, "commit", "-qam", "main change")
	head := gitRun(t, repo, "rev-parse", "HEAD")
	if err := Move(scope, id, "done", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := Get(scope, id)
	if got.Status != "blocked" {
		t.Fatalf("status %q, want blocked", got.Status)
	}
	if n := len(got.Comments); n == 0 || !strings.HasPrefix(got.Comments[n-1].Body, "merge failed: ") {
		t.Fatalf("no merge failure comment: %+v", got.Comments)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatal("worktree removed after failed merge")
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "base.txt")); string(b) != "main\n" {
		t.Fatalf("main changed: %q", b)
	}
	if gitRun(t, repo, "rev-parse", "HEAD") != head {
		t.Fatal("main HEAD moved")
	}
}

func TestDoneKeepsUnrelatedMainChanges(t *testing.T) {
	repo, scope, id, wt := newMergeRepo(t)
	os.WriteFile(filepath.Join(wt, "fix.txt"), []byte("fix\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "base.txt"), []byte("local\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "scratch.txt"), []byte("scratch\n"), 0o644)
	if err := Move(scope, id, "done", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(repo, "fix.txt")); err != nil {
		t.Fatalf("work not merged: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "base.txt")); string(b) != "local\n" {
		t.Fatalf("unrelated change touched: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(repo, "scratch.txt")); string(b) != "scratch\n" {
		t.Fatalf("untracked file touched: %q", b)
	}
}

func TestDoneWithoutWorktreeIsNoop(t *testing.T) {
	_, scope, _, _ := newMergeRepo(t)
	task, err := Add(scope, "no worktree", nil)
	if err != nil {
		t.Fatal(err)
	}
	id := task.ID
	if err := Move(scope, id, "done", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := Get(scope, id)
	if got.Status != "done" || len(got.Comments) != 0 {
		t.Fatalf("status %q comments %+v", got.Status, got.Comments)
	}
}
