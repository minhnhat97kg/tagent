package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workplace"
)

// A ticket moved to done must remove its clean workflow worktree and branch.
func TestTaskDoneRemovesCleanWorkflowWorktree(t *testing.T) {
	repo := t.TempDir()
	t.Chdir(repo)
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"AUTHOR", "COMMITTER"} {
		t.Setenv("GIT_"+k+"_NAME", "test")
		t.Setenv("GIT_"+k+"_EMAIL", "test@test")
	}
	for _, args := range [][]string{{"init", "-q"}, {"commit", "--allow-empty", "-qm", "base"}} {
		if _, err := git(repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	ticket, err := tasks.Add(tasks.Scope(repo), "ticket", nil)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := workflowWorktree(repo, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := cmdTask([]string{"move", ticket.ID, "done"}, &out, &errs); code != exitOK {
		t.Fatal(errs.String())
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree %s kept after done: %v", wt, err)
	}
	if b, _ := git(repo, "branch", "--list", "agent/workflow-"+ticket.ID); b != "" {
		t.Fatalf("branch kept: %s", b)
	}
}

func TestGCSelect(t *testing.T) {
	live := func(p string) bool { return p == "/alive" }
	agent := func(st tmux.PaneStatus) []sessionPane { return []sessionPane{{Agent: "pi", Status: st}} }
	in := []sessionInfo{
		{Name: "mosh", Panes: agent(tmux.StatusIdle)},                          // unmanaged
		{Name: "gone-idle", Owner: "/gone", Panes: agent(tmux.StatusIdle)},     // orphan
		{Name: "gone-busy", Owner: "/gone", Panes: agent(tmux.StatusRunning)},  // orphan but running
		{Name: "gone-wait", Owner: "/gone", Panes: agent(tmux.StatusWaiting)},  // orphan but waiting
		{Name: "ok", Owner: "/alive", Panes: agent(tmux.StatusIdle)},           // healthy
		{Name: "empty", Owner: "/alive", Panes: []sessionPane{{Window: "@1"}}}, // no agent left
		{Name: "vim", Owner: "/alive", Panes: []sessionPane{{Cmd: "vim"}}},     // no agent, live editor
		{Name: "gone-unmanaged", Panes: nil},                                   // unmanaged, no panes
	}
	var got []string
	for _, s := range gcSelect(in, live) {
		got = append(got, s.Name)
	}
	if strings.Join(got, ",") != "gone-idle,empty" {
		t.Fatalf("gc selected %v", got)
	}
}

func TestCleanupKeepsDirtyAndUnmerged(t *testing.T) {
	repo := t.TempDir()
	for _, k := range []string{"AUTHOR", "COMMITTER"} {
		t.Setenv("GIT_"+k+"_NAME", "test")
		t.Setenv("GIT_"+k+"_EMAIL", "test@test")
	}
	for _, args := range [][]string{{"init", "-q"}, {"commit", "--allow-empty", "-qm", "base"}} {
		if _, err := git(repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	dirty, err := workflowWorktree(repo, "7")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dirty, "wip"), []byte("x"), 0600)
	if notes := (workplace.Git{}).Cleanup(repo, "7"); len(notes) == 0 {
		t.Fatal("dirty worktree removed silently")
	}
	if _, err := os.Stat(dirty); err != nil {
		t.Fatal("dirty worktree was removed")
	}
	// Clean but with an unmerged commit: worktree goes, branch stays.
	clean, err := workflowWorktree(repo, "8")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := git(clean, "commit", "--allow-empty", "-qm", "work"); err != nil {
		t.Fatal(err)
	}
	notes := workplace.Git{}.Cleanup(repo, "8")
	if _, err := os.Stat(clean); !os.IsNotExist(err) {
		t.Fatal("clean worktree kept")
	}
	if b, _ := git(repo, "branch", "--list", "agent/workflow-8"); b == "" || len(notes) != 1 {
		t.Fatalf("unmerged branch must stay with a note: %q %v", b, notes)
	}
}
