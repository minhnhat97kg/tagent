package main

import (
	"os"
	"path/filepath"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/workplace"
)

// workflowWorktree is the git-worktree workplace's Create, as the test helper it was.
func workflowWorktree(dir, id string) (string, error) {
	return workplace.Git{}.Create(tasks.MainRoot(dir), id)
}

func TestWorkflowWorktreeIsolation(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@test")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@test")
	for _, args := range [][]string{{"init", "-q"}, {"commit", "--allow-empty", "-qm", "baseline"}} {
		if _, err := git(repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	first, err := workflowWorktree(repo, "2")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(first, "handoff")
	if err = os.WriteFile(file, []byte("job A"), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := workflowWorktree(first, "2")
	if err != nil || again != first {
		t.Fatalf("reuse: %s %v", again, err)
	}
	data, err := os.ReadFile(filepath.Join(again, "handoff"))
	if err != nil || string(data) != "job A" {
		t.Fatalf("handoff: %s %v", data, err)
	}
	if err = os.WriteFile(file, append(data, []byte(" job B")...), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := workflowWorktree(repo, "3")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{repo, second} {
		if _, err = os.Stat(filepath.Join(dir, "handoff")); !os.IsNotExist(err) {
			t.Fatalf("leaked into %s: %v", dir, err)
		}
	}
	collision := filepath.Join(repo, ".worktrees", "workflow-4")
	if err = os.MkdirAll(collision, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = workflowWorktree(repo, "4"); err == nil {
		t.Fatal("accepted unregistered directory")
	}
	if _, err = workflowWorktree(repo, "../escape"); err == nil {
		t.Fatal("accepted invalid ID")
	}
	if _, err = git(repo, "worktree", "remove", "--force", second); err != nil {
		t.Fatal(err)
	}
	if _, err = workflowWorktree(repo, "3"); err == nil {
		t.Fatal("recreated removed tree")
	}
	// A registered path is not sufficient: its .git file may have been replaced.
	for _, id := range []string{"5", "6"} {
		path, err := workflowWorktree(repo, id)
		if err != nil {
			t.Fatal(err)
		}
		if id == "5" {
			if err = os.Remove(filepath.Join(path, ".git")); err != nil {
				t.Fatal(err)
			}
			if _, err = git(path, "init", "-q", "-b", "agent/workflow-5"); err != nil {
				t.Fatal(err)
			}
			if _, err = git(path, "commit", "--allow-empty", "-qm", "foreign"); err != nil {
				t.Fatal(err)
			}
		} else {
			// Leave the expected branch symbolic ref pointing to an unborn branch.
			if _, err = git(path, "update-ref", "-d", "refs/heads/agent/workflow-6"); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = workflowWorktree(repo, id); err == nil {
			t.Fatalf("accepted corrupt worktree %s", id)
		}
	}
}
