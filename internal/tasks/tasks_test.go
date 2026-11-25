package tasks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoardSharedAcrossWorktrees(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", append([]string{"-C", repo}, args...)...)
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if b, e := c.CombinedOutput(); e != nil {
			t.Fatalf("git %v: %s", args, b)
		}
	}
	git("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(repo, "a"), []byte("x"), 0644)
	git("add", "a")
	git("commit", "-qm", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	git("worktree", "add", "-q", "-b", "worker", wt)
	canonicalRepo, _ := filepath.EvalSymlinks(repo)
	if BoardConfigPath(wt) != filepath.Join(canonicalRepo, ".tasks", "board.yaml") {
		t.Fatal("config must use main worktree")
	}
	if err := SaveBoardConfig(wt, []byte("columns: [todo, custom, done]")); err != nil {
		t.Fatal(err)
	}
	c, err := LoadBoardConfig(repo)
	if err != nil || c.Columns[1] != "custom" {
		t.Fatal(c, err)
	}
	// Restore ordinary columns for the existing status checks below.
	SaveBoardConfig(repo, []byte("columns: [todo, doing, done]"))
	scope := Scope(repo)
	if Scope(wt) != scope {
		t.Fatal("worktree must share the task board")
	}
	task, err := Add(scope, "write tests", []string{"tester"})
	if err != nil {
		t.Fatal(err)
	}
	if got := List(Scope(wt)); len(got) != 1 || got[0].Text != "write tests" {
		t.Fatalf("worktree tasks: %+v", got)
	}
	updated, err := Update(scope, task.ID, "doing", []string{"tester", "reviewer"})
	if err != nil || updated.Status != "doing" || len(updated.Assignees) != 2 {
		t.Fatalf("updated: %+v %v", updated, err)
	}
	if _, err := Update(scope, task.ID, "invalid", nil); err == nil {
		t.Fatal("invalid status accepted")
	}
	if err := Remove(scope, task.ID); err != nil || len(List(scope)) != 0 {
		t.Fatal("remove failed")
	}
}

func TestShortSequentialIDs(t *testing.T) {
	scope := t.TempDir()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	a, err := Add(scope, "one", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Add(scope, "two", nil)
	if a.ID != "1" || b.ID != "2" {
		t.Fatalf("ids %q %q", a.ID, b.ID)
	}
	if err := Remove(scope, b.ID); err != nil {
		t.Fatal(err)
	}
	if c, _ := Add(scope, "three", nil); c.ID != "3" {
		t.Fatalf("deleted id reused: %q", c.ID)
	}
}

func TestTitleOfCapsLongFirstLine(t *testing.T) {
	if got := TitleOf("Short title\n\nDetails"); got != "Short title" {
		t.Fatalf("got %q", got)
	}
	long := "Token saver: install.sh/tagent install also installs rtk (output-compressing CLI proxy) and wires it into each harness"
	got := TitleOf(long)
	if n := len([]rune(got)); n > 80 || !strings.HasSuffix(got, "…") || !strings.HasPrefix(long, strings.TrimSuffix(got, "…")) {
		t.Fatalf("got %q (%d runes)", got, n)
	}
}
