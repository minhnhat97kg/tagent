package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"tagent/internal/tasks"
	"testing"
)

func TestTaskDispatchVisibility(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	t.Chdir(dir)
	scope := tasks.Scope(dir)
	a, _ := tasks.Add(scope, "first", nil)
	b, _ := tasks.Add(scope, "second", nil)
	for _, flag := range []string{"", "--all", "--brief"} {
		var out, stderr bytes.Buffer
		args := []string{"show", a.ID}
		if flag != "" {
			args = append(args, flag)
		}
		if code := cmdTask(args, &out, &stderr); code != exitOK || strings.Contains(out.String(), "Dispatch:") {
			t.Fatal(code, out.String(), stderr.String())
		}
	}
	tasks.DispatchAudit(scope, b.ID, "secret-other-ticket")
	tasks.DispatchAudit(scope, a.ID, "old-outcome")
	tasks.DispatchAudit(scope, a.ID, "failure: "+strings.Repeat("x", 1000))
	for _, flag := range []string{"", "--all", "--brief"} {
		var out, stderr bytes.Buffer
		args := []string{"show", a.ID}
		if flag != "" {
			args = append(args, flag)
		}
		if code := cmdTask(args, &out, &stderr); code != exitOK {
			t.Fatal(code, stderr.String())
		}
		s := out.String()
		if !strings.Contains(s, "failure:") || strings.Contains(s, "secret-other-ticket") {
			t.Fatal(s)
		}
		if flag == "--brief" && (strings.Contains(s, "old-outcome") || len(s) > 900) {
			t.Fatal(s)
		}
	}
	for _, args := range [][]string{{"retry"}, {"retry", a.ID, "extra"}} {
		if code := cmdTask(args, &bytes.Buffer{}, &bytes.Buffer{}); code != exitUsage {
			t.Fatal(code)
		}
	}
	if code := cmdTask([]string{"retry", a.ID}, &bytes.Buffer{}, &bytes.Buffer{}); code == exitOK {
		t.Fatal("role-less retry accepted")
	}
	if err := os.WriteFile(filepath.Join(tasks.BoardDir(scope), "events.jsonl"), []byte("broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := cmdTask([]string{"show", a.ID}, &bytes.Buffer{}, &bytes.Buffer{}); code == exitOK {
		t.Fatal("history error hidden")
	}
}
