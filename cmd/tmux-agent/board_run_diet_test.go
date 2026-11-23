package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tmux-agent/internal/board"
	"tmux-agent/internal/tasks"
)

func TestCISummaryAndAttachment(t *testing.T) {
	s := ciSummary(strings.NewReader("--- FAIL: early\nok\tpkg\n?\tpkg2\n" + strings.Repeat("progress\n", 30) + "x.go:4: error\n"))
	if strings.Contains(s, "ok\t") || strings.Contains(s, "?\t") || !strings.Contains(s, "--- FAIL") || !strings.Contains(s, "x.go:4") || len(strings.Split(s, "\n")) > 12 {
		t.Fatal(s)
	}
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".tasks"), 0700)
	os.WriteFile(filepath.Join(dir, ".tasks", "board.yaml"), []byte("statuses:\n - {id: todo}\n - {id: test, run: x, pass: done, fail: blocked}\n - {id: blocked}\n - {id: done}\n"), 0600)
	scope := tasks.Scope(dir)
	task, e := tasks.Add(scope, "ci", nil)
	if e != nil {
		t.Fatal(e)
	}
	tasks.Move(scope, task.ID, "test", "")
	ev, _ := tasks.ReadEvents(scope, 0)
	if e = runStatus(board.Action{Dir: dir, Run: "printf 'ok\\tpkg\\n--- FAIL: broken\\n'; exit 1", Pass: "done", Fail: "blocked", Event: ev[len(ev)-1]}); e != nil {
		t.Fatal(e)
	}
	got, _ := tasks.Get(scope, task.ID)
	c := got.Comments[len(got.Comments)-1]
	if !strings.Contains(c.Body, "--- FAIL") || got.Status != "blocked" {
		t.Fatal(got)
	}
	found := false
	for _, a := range c.Attachments {
		if a.Name == "ci.log" {
			found = true
			f, e := tasks.OpenAttachment(scope, task.ID, a.File)
			if e != nil {
				t.Fatal(e)
			}
			b, _ := io.ReadAll(f)
			f.Close()
			if !strings.Contains(string(b), "ok\tpkg") {
				t.Fatal(string(b))
			}
		}
	}
	if !found {
		t.Fatal(c)
	}
}
