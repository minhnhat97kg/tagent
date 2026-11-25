package tasks

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/tmux"
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
	if err := Move(scope, id, "done", ""); err == nil || !strings.Contains(err.Error(), "merge failed") {
		t.Fatalf("done on conflict: %v", err)
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

// #79 regression: a workflow's done has no next status, so the old done -> blocked
// fallback was refused and the ticket stayed done with its notice already sent.
func TestDoneConflictOnWorkflowTicketBlocks(t *testing.T) {
	repo, scope, _, _ := newMergeRepo(t)
	dir := filepath.Join(repo, ".tasks", "workflows")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "feature.yml"), []byte("name: feature\ninputs:\n  goal: {required: true}\njobs:\n  build: {status: building, agent: implementer}\n"), 0o600)
	if err := SaveBoardConfig(repo, []byte("roles: {implementer: {agent: pi}}\ndefault_workflow: feature\n")); err != nil {
		t.Fatal(err)
	}
	ticket, err := Add(scope, "Fix the thing", nil, MutationOptions{Workflow: "feature", Inputs: map[string]string{"goal": "fix"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := Move(scope, ticket.ID, "building", ""); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(repo, ".worktrees", "workflow-"+ticket.ID)
	gitRun(t, repo, "worktree", "add", "-q", "-b", "agent/workflow-"+ticket.ID, wt)
	os.WriteFile(filepath.Join(wt, "base.txt"), []byte("worktree\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "base.txt"), []byte("main\n"), 0o644)
	gitRun(t, repo, "commit", "-qam", "main change")

	messages := captureNotices(t)
	err = Move(scope, ticket.ID, "done", "")
	if err == nil || !strings.Contains(err.Error(), "merge failed") {
		t.Fatalf("done on conflict: %v", err)
	}
	got, _ := Get(scope, ticket.ID)
	if got.Status != "blocked" || !got.CompletedAt.IsZero() {
		t.Fatalf("status %q completed %v", got.Status, got.CompletedAt)
	}
	history, _ := ReadEvents(scope, 0)
	for _, ev := range history {
		if ev.Task == ticket.ID && ev.Kind == "status" && ev.To == "done" {
			t.Fatal("done event recorded")
		}
	}
	if n := *messages; len(n) != 1 || !strings.Contains(n[0], "blocked") || !strings.Contains(n[0], "merge failed") {
		t.Fatalf("notices %q", n)
	}
}

// captureNotices records the status notices sent while the test runs.
func captureNotices(t *testing.T) *[]string {
	t.Helper()
	fake := tmux.NewFake()
	t.Cleanup(tmux.SetRunner(fake))
	var messages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		messages = append(messages, string(b))
	}))
	t.Cleanup(srv.Close)
	fake.Set(srv.URL, "show", "-gv", "@agent_notify_url")
	return &messages
}

// makeConflict edits base.txt in the ticket's worktree and in main, so the merge conflicts.
func makeConflict(t *testing.T, repo, wt string) {
	t.Helper()
	os.WriteFile(filepath.Join(wt, "base.txt"), []byte("worktree\n"), 0o644)
	os.WriteFile(filepath.Join(repo, "base.txt"), []byte("main\n"), 0o644)
	gitRun(t, repo, "commit", "-qam", "main change")
}

func TestDoneConflictThroughUpdateBlocks(t *testing.T) {
	repo, scope, id, wt := newMergeRepo(t)
	makeConflict(t, repo, wt)
	got, err := Update(scope, id, "done", nil)
	if err == nil || !strings.Contains(err.Error(), "merge failed") || got.Status != "blocked" {
		t.Fatalf("update done on conflict: %v, status %q", err, got.Status)
	}
}

func TestDoneWhileBlockedAddsOnlyCommentAndErrors(t *testing.T) {
	repo, scope, id, wt := newMergeRepo(t)
	makeConflict(t, repo, wt)
	if err := Move(scope, id, "done", ""); err == nil {
		t.Fatal("conflict accepted")
	}
	statusEvents := func() int {
		history, _ := ReadEvents(scope, 0)
		n := 0
		for _, ev := range history {
			if ev.Task == id && ev.Kind == "status" {
				n++
			}
		}
		return n
	}
	before := statusEvents()
	if err := Move(scope, id, "done", ""); err == nil || !strings.Contains(err.Error(), "merge failed") {
		t.Fatalf("done again: %v", err)
	}
	got, _ := Get(scope, id)
	if got.Status != "blocked" || statusEvents() != before {
		t.Fatalf("status %q, status events %d -> %d", got.Status, before, statusEvents())
	}
}

func TestDoneAfterResolvingConflictNotesAgentSummary(t *testing.T) {
	repo, scope, id, wt := newMergeRepo(t)
	makeConflict(t, repo, wt)
	if err := Move(scope, id, "done", ""); err == nil {
		t.Fatal("conflict accepted")
	}
	// Resolve by making main take the worktree's change, which the failed merge already committed.
	os.WriteFile(filepath.Join(repo, "base.txt"), []byte("worktree\n"), 0o644)
	gitRun(t, repo, "commit", "-qam", "main takes the worktree change")
	messages := captureNotices(t)
	if err := Move(scope, id, "done", "agent summary"); err != nil {
		t.Fatal(err)
	}
	if got, _ := Get(scope, id); got.Status != "done" {
		t.Fatalf("status %q, want done", got.Status)
	}
	if n := *messages; len(n) != 1 || !strings.Contains(n[0], "agent summary") || strings.Contains(n[0], "merged ") {
		t.Fatalf("notices %q", n)
	}
}
