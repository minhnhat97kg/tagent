package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/tasks"
)

func TestTaskCommentDiffShowAndBoardInit(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	t.Chdir(dir)
	git := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		b, e := c.CombinedOutput()
		if e != nil {
			t.Fatal(e, string(b))
		}
		return string(b)
	}
	git("init", "-q")
	os.WriteFile("a", []byte("old\n"), 0600)
	git("add", "a")
	git("commit", "-qm", "initial")
	os.WriteFile("a", []byte("new\n"), 0600)
	run := func(args ...string) string {
		t.Helper()
		var out, stderr bytes.Buffer
		if code := cmdTask(args, &out, &stderr); code != exitOK {
			t.Fatalf("%v: %d %s", args, code, stderr.String())
		}
		return out.String()
	}
	id := strings.TrimSpace(run("add", "Title\nBody"))
	run("due", id, "2026-12-01")
	run("comment", id, "--diff", "hello @planner")
	task, err := tasks.Get(tasks.Scope(dir), id)
	if err != nil {
		t.Fatal(err)
	}
	a := task.Comments[0].Attachments[0]
	if a.Name != "diff.patch" {
		t.Fatal(a)
	}
	b, err := os.ReadFile(filepath.Join(tasks.BoardDir(tasks.Scope(dir)), id, a.File))
	if err != nil || string(b) != git("diff", "HEAD") {
		t.Fatal(string(b), err)
	}
	shown := run("show", id)
	for _, want := range []string{"Title", "Body", "2026-12-01", "hello @planner", "diff.patch"} {
		if !strings.Contains(shown, want) {
			t.Fatalf("missing %s: %s", want, shown)
		}
	}
	run("due", id, "-")
	task, _ = tasks.Get(tasks.Scope(dir), id)
	if task.Due != "" {
		t.Fatal(task.Due)
	}
	var out, stderr bytes.Buffer
	if code := cmdBoard([]string{"init", "--dir", dir}, &out, &stderr); code != exitOK {
		t.Fatal(stderr.String())
	}
	if _, err = tasks.LoadBoardConfig(dir); err != nil {
		t.Fatal(err)
	}
	if code := cmdBoard([]string{"init", "--dir", dir}, &out, &stderr); code != exitFailure {
		t.Fatal("init overwrote config")
	}
	if err = tasks.SetWorkflow(tasks.Scope(dir), id, "feature", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = tasks.Update(tasks.Scope(dir), id, "build", nil); err != nil {
		t.Fatal(err)
	}
}
