package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"tagent/internal/tasks"
	"testing"
)

func TestTaskAttachmentsCommand(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)
	task, err := tasks.Add(tasks.Scope(dir), "Files", nil)
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := cmdTask([]string{"attachments"}, &out, &stderr); code != exitUsage {
		t.Fatal(code)
	}
	out.Reset()
	stderr.Reset()
	if code := cmdTask([]string{"attachments", task.ID}, &out, &stderr); code != exitOK || out.Len() != 0 {
		t.Fatal(code, out.String(), stderr.String())
	}
	p := filepath.Join(dir, "ci.log")
	os.WriteFile(p, []byte("passed"), 0600)
	if err = tasks.Comment(tasks.Scope(dir), task.ID, "ci", "", []string{p}); err != nil {
		t.Fatal(err)
	}
	if code := cmdTask([]string{"attachments", task.ID}, &out, &stderr); code != exitOK || !strings.Contains(out.String(), "ci.log") || !strings.Contains(out.String(), "6 bytes") {
		t.Fatal(code, out.String(), stderr.String())
	}
	if code := cmdTask([]string{"attachments", "../escape"}, &out, &stderr); code == exitOK {
		t.Fatal("invalid id")
	}
}
