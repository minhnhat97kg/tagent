package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"tmux-agent/internal/tasks"
)

func TestTaskStartWorkflow(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "sample.yml"), []byte("name: sample\njobs:\n  build: {run: echo ok}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "ticket", nil)
	if err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := cmdTask([]string{"start", ticket.ID, "sample"}, &out, &errs); code != exitOK {
		t.Fatal(errs.String())
	}
	got, _ := tasks.Get(scope, ticket.ID)
	if got.Workflow != "sample" || got.Status != "build" {
		t.Fatal(got)
	}
}
