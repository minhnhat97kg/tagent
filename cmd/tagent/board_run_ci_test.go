package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/board"
	"tagent/internal/tasks"
)

// A ticket's test run that fails only on tests which also fail on main moves
// forward with a "pre-existing" comment; a test that fails only on the ticket
// goes to fail.
func TestRunStatusPreExistingForwardsThroughGit(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com"}, args...)
		if b, err := exec.Command("git", full...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, b)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	failing := func(name string) string {
		return "package ci\n\nimport \"testing\"\n\nfunc " + name + "(t *testing.T) { t.Fatal(\"boom\") }\n"
	}
	git("init", "-q", "-b", "main")
	write("go.mod", "module example.com/ci\n\ngo 1.21\n")
	write("a_test.go", failing("TestA"))
	git("add", ".")
	git("commit", "-q", "-m", "main: TestA fails")
	git("checkout", "-q", "-b", "ticket")

	if err := tasks.SaveBoardConfig(repo, []byte("statuses: [{id: todo}, {id: test, run: go test ./..., pass: done, fail: todo}, {id: done}]")); err != nil {
		t.Fatal(err)
	}
	scope := tasks.Scope(repo)
	// runTicket moves a fresh ticket into test and runs its test job in the repo.
	runTicket := func() tasks.Task {
		t.Helper()
		task, _ := tasks.Add(scope, "CI ticket", nil)
		if err := tasks.Move(scope, task.ID, "test", ""); err != nil {
			t.Fatal(err)
		}
		events, _ := tasks.ReadEvents(scope, 0)
		entry := events[len(events)-1]
		if err := runStatus(board.Action{Dir: repo, ExecDir: repo, Event: entry, Run: "go test ./...", Pass: "done", Fail: "todo"}); err != nil {
			t.Fatal(err)
		}
		got, _ := tasks.Get(scope, task.ID)
		return got
	}

	// Only TestA fails, and it fails on main too: forward with a comment naming it.
	got := runTicket()
	if got.Status != "done" || len(got.Comments) != 1 || !strings.HasPrefix(got.Comments[0].Body, "pre-existing: TestA\n") {
		t.Fatalf("pre-existing failure: status=%s comments=%+v", got.Status, got.Comments)
	}

	// TestB fails only on the ticket: it is the ticket's failure, back to dev.
	write("b_test.go", failing("TestB"))
	git("add", ".")
	git("commit", "-q", "-m", "ticket: TestB fails")
	got = runTicket()
	if got.Status != "todo" || len(got.Comments) != 1 || strings.Contains(got.Comments[0].Body, "pre-existing") {
		t.Fatalf("ticket failure: status=%s comments=%+v", got.Status, got.Comments)
	}
}
