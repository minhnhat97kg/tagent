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

// Prune removes only clean, merged checkouts of done or ticket-less tickets; a dry run removes nothing.
func TestWorkplacePrune(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@test")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@test")
	defer tmux.SetRunner(tmux.NewFake())()
	repo, _ := filepath.EvalSymlinks(t.TempDir())
	for _, args := range [][]string{{"init", "-q"}, {"commit", "--allow-empty", "-qm", "baseline"}} {
		if _, err := git(repo, args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(repo)
	scope := tasks.Scope(repo)
	add := func(status string) string {
		task, err := tasks.Add(scope, "ticket", nil)
		if err != nil {
			t.Fatal(err)
		}
		if status == "done" {
			if err := tasks.Move(scope, task.ID, "done", ""); err != nil {
				t.Fatal(err)
			}
		}
		return task.ID
	}
	merged, dirty, unmerged, todo, orphan, half := add("done"), add("done"), add("done"), add("todo"), add("done"), add("done")
	missing := "9001" // no ticket record at all
	wt := func(id string) string { return filepath.Join(repo, workplace.Dir, "workflow-"+id) }
	for _, id := range []string{merged, dirty, unmerged, todo, missing, half} {
		if _, err := (workplace.Git{}).Create(repo, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := git(wt(unmerged), "commit", "--allow-empty", "-qm", "work"); err != nil {
		t.Fatal(err)
	}
	// half: merged, then tracked files lost from disk by an interrupted removal
	if err := os.WriteFile(filepath.Join(wt(half), "f.txt"), []byte("f\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := git(wt(half), "add", "f.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := git(wt(half), "commit", "-qm", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := git(repo, "merge", "--ff-only", "-q", "agent/workflow-"+half); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(wt(half), "f.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt(dirty), "scratch.txt"), []byte("scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wt(orphan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt(orphan), ".git"), []byte("gitdir: /nowhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errs bytes.Buffer
	if code := cmdWorkplace([]string{"prune"}, &out, &errs); code != exitOK {
		t.Fatalf("dry run code %d: %s", code, errs.String())
	}
	for _, id := range []string{merged, missing, half, orphan} {
		if !strings.Contains(out.String(), "remove "+wt(id)) {
			t.Fatalf("dry run does not list %s as remove:\n%s", id, out.String())
		}
	}
	if !strings.Contains(out.String(), "remove "+wt(orphan)+" (ticket done, merged, unusable dir (untracked files not checked))") {
		t.Fatalf("orphan dir not labelled unusable:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "dry run") {
		t.Fatalf("dry run output:\n%s", out.String())
	}
	for _, id := range []string{merged, missing, dirty, unmerged, todo, orphan, half} {
		if _, err := os.Stat(wt(id)); err != nil {
			t.Fatalf("dry run removed %s", id)
		}
	}

	out.Reset()
	if code := cmdWorkplace([]string{"prune", "--yes"}, &out, &errs); code != exitOK {
		t.Fatalf("prune code %d: %s\n%s", code, errs.String(), out.String())
	}
	for _, want := range []string{
		"skip " + wt(dirty) + " (uncommitted changes)",
		"skip " + wt(unmerged) + " (not merged)",
		"skip " + wt(todo) + " (ticket is todo)",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in:\n%s", want, out.String())
		}
	}
	for _, id := range []string{merged, missing, half, orphan} {
		if _, err := os.Stat(wt(id)); !os.IsNotExist(err) {
			t.Fatalf("prune kept %s", id)
		}
		if b, _ := git(repo, "branch", "--list", "agent/workflow-"+id); b != "" {
			t.Fatalf("branch kept for %s: %q", id, b)
		}
	}
	for _, id := range []string{dirty, unmerged, todo} {
		if _, err := os.Stat(wt(id)); err != nil {
			t.Fatalf("prune removed %s", id)
		}
	}
}
