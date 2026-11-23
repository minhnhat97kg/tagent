package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"tagent/internal/tasks"
	"tagent/internal/workflow"
	"testing"
)

func TestIntakeApprovedWorkflowInput(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.MkdirAll(filepath.Join(dir, ".tasks", "workflows"), 0755)
	if e := os.WriteFile(filepath.Join(dir, ".tasks", "workflows", "build.yml"), []byte(workflow.Template("build")), 0600); e != nil {
		t.Fatal(e)
	}
	old := intakeCommand
	defer func() { intakeCommand = old }()
	intakeCommand = func() string {
		return "printf 'TICKET\\ntitle: Build feature\\ngoal: Works\\nacceptance:\\n- Tests pass\\nout of scope:\\n- Other\\nworkflow: build'"
	}
	var out, errs bytes.Buffer
	if code := cmdTaskIntake([]string{"-i", "idea"}, strings.NewReader("y\n"), &out, &errs); code != exitOK {
		t.Fatal(code, errs.String())
	}
	ts := tasks.List(tasks.Scope(dir))
	if len(ts) != 1 || ts[0].Workflow != "build" || ts[0].Inputs["goal"] != ts[0].Text || ts[0].Status != "todo" {
		t.Fatal(ts, errs.String())
	}
}
