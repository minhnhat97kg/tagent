package board

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"tagent/internal/tasks"
)

func has(dir string) bool { return slices.Contains(Workspaces(), tasks.MainRoot(dir)) }

func TestWorkspaceAddRemove(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir := t.TempDir()
	root, err := AddWorkspace(dir)
	if err != nil || root != tasks.MainRoot(dir) || !has(dir) {
		t.Fatalf("add plain folder: %q %v", root, err)
	}
	if err := RemoveWorkspace(dir); err != nil || has(dir) {
		t.Fatalf("remove: %v, still listed=%v", err, has(dir))
	}
	if err := Register(dir); err != nil || has(dir) {
		t.Fatalf("Register must not unhide: listed=%v", has(dir))
	}
	if _, err := AddWorkspace(dir); err != nil || !has(dir) {
		t.Fatalf("re-add: %v", err)
	}
}

func TestWorkspaceRemoveNeverRegistered(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir := t.TempDir()
	if err := RemoveWorkspace(dir); err != nil || has(dir) {
		t.Fatalf("remove unregistered: %v", err)
	}
	if len(Registered()) != 0 {
		t.Fatal("remove must not register")
	}
}

func TestWorkspaceWorktreeListsMainRepo(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	repo := t.TempDir()
	wt := filepath.Join(t.TempDir(), "wt")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", "init")
	git("worktree", "add", "-q", wt)
	root, err := AddWorkspace(wt)
	if err != nil || root != tasks.MainRoot(repo) {
		t.Fatalf("worktree -> %q (%v), want main repo %q", root, err, tasks.MainRoot(repo))
	}
}

func TestWorkspaceRejectsBadPaths(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	file := filepath.Join(t.TempDir(), "f")
	os.WriteFile(file, nil, 0600)
	for _, p := range []string{"relative/dir", filepath.Join(t.TempDir(), "missing"), file, ""} {
		if _, err := AddWorkspace(p); err == nil {
			t.Errorf("AddWorkspace(%q) accepted", p)
		}
		if err := RemoveWorkspace(p); err == nil {
			t.Errorf("RemoveWorkspace(%q) accepted", p)
		}
	}
}

func TestWorkspaceTildeIsHome(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := AddWorkspace("~/"); err != nil || !has(home) {
		t.Fatalf("~ add: %v", err)
	}
}
