package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"tagent/internal/board"
	"tagent/internal/queue"
	"tagent/internal/roles"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"testing"
	"time"
)

func TestBoardAgentLaunchReuseAndQueue(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux unavailable")
	}
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	fake := filepath.Join(dir, "claude")
	os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$BOARD_TEST_LAUNCHES\"\ntmux set-option -p -t \"$TMUX_PANE\" @pane_agent claude\ntmux set-option -p -t \"$TMUX_PANE\" @pane_status idle\nwhile IFS= read -r line; do printf '%s\\n' \"$line\" >> \"$BOARD_TEST_RECEIVED\"; done\n"), 0700)
	received := filepath.Join(dir, "received")
	t.Setenv("BOARD_TEST_RECEIVED", received)
	launches := filepath.Join(dir, "launches")
	t.Setenv("BOARD_TEST_LAUNCHES", launches)
	if b, err := exec.Command("tmux", "-f", "/dev/null", "new-session", "-d", "-c", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(b))
	}
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })
	tmux.RunTmux("set-option", "-g", "@agent_scrape", "off")
	os.MkdirAll(roles.GlobalDir(), 0700)
	os.WriteFile(filepath.Join(roles.GlobalDir(), "planner.md"), []byte("Plan carefully."), 0600)
	os.WriteFile(filepath.Join(roles.GlobalDir(), "profiles.toml"), []byte("[planner]\nharness = '"+fake+"'\n"), 0600)
	a := board.Action{Dir: dir, Agent: "planner", Prompt: "first prompt", Effort: "high"}
	if err := dispatchBoardAction(a); err != nil {
		t.Fatal(err)
	}
	pane := tmux.ResolvePane("planner")
	if pane == "planner" {
		t.Fatal("no pane")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		b, _ := os.ReadFile(received)
		if strings.Contains(string(b), "first prompt") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("delivery timeout")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The role's app-owned instructions ride in the dispatch prompt for claude.
	if got, _ := os.ReadFile(received); !strings.Contains(string(got), "Plan carefully.") {
		t.Fatalf("instructions not dispatched: %s", got)
	}
	b, err := os.ReadFile(launches)
	if err != nil || !strings.Contains(string(b), "--effort high") || strings.Contains(string(b), "Plan carefully.") {
		t.Fatalf("launch arguments: %s (%v)", b, err)
	}
	a.Effort = "low" // Reuse must not reconfigure or relaunch the pane.
	for _, status := range []string{"running", "waiting", ""} {
		tmux.SetPaneOption(pane, tmux.PaneStatusOpt, status)
		if err := dispatchBoardAction(a); err != nil {
			t.Fatal(err)
		}
	}
	b, _ = os.ReadFile(launches)
	if strings.Count(string(b), "\n") != 1 {
		t.Fatalf("relaunch: %s", b)
	}
	if len(queue.List(pane)) != 3 {
		t.Fatal("missing queued prompts")
	}
	if tmux.ResolvePane("planner-2") != "planner-2" {
		t.Fatal("duplicate pane")
	}
	ticket, err := tasks.Add(tasks.Scope(dir), "board launch", nil)
	if err != nil {
		t.Fatal(err)
	}
	a.Event.Task = ticket.ID
	for range 2 {
		if err := dispatchBoardAction(a); err != nil {
			t.Fatal(err)
		}
	}
	name := "planner-" + ticket.ID[max(0, len(ticket.ID)-4):]
	if tmux.ResolvePane(name) == name || tmux.ResolvePane(name+"-2") != name+"-2" {
		t.Fatal("task pane identity")
	}
	a.ResumePrompt = func(int) string { return "delta" }
	a.FreshAfter = 3
	previous := tmux.ResolvePane(name)
	for entry := 1; entry <= 5; entry++ {
		tmux.SetPaneOption(previous, tmux.PaneStatusOpt, "running")
		if err := dispatchBoardAction(a); err != nil {
			t.Fatal(err)
		}
		current := tmux.ResolvePane(name)
		rotated := current != previous
		if rotated != (entry == 3 || entry == 5) {
			t.Fatalf("entry %d rotation=%v", entry, rotated)
		}
		want := "2"
		if entry%2 == 1 {
			want = "1"
		}
		if got := tmux.GetPaneOptionValue(current, "@agent_role_entries"); got != want {
			t.Fatalf("entry %d count %s", entry, got)
		}
		if rotated {
			if len(queue.List(previous)) != 0 {
				t.Fatal("stale queue")
			}
			panes, err := tmux.RunTmuxCapture("list-panes", "-a", "-F", "#{pane_id}")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range strings.Fields(panes) {
				if id == previous {
					t.Fatal("old pane survives")
				}
			}
		}
		previous = current
	}
	for _, tc := range []struct {
		threshold int
		metadata  string
		rotate    bool
	}{
		{0, "9", false}, {1, "9", true}, {3, "invalid", false},
		{3, "", false}, {2, "1", true},
	} {
		a.FreshAfter = tc.threshold
		tmux.SetPaneOption(previous, "@agent_role_entries", tc.metadata)
		tmux.SetPaneOption(previous, tmux.PaneStatusOpt, "running")
		if err := dispatchBoardAction(a); err != nil {
			t.Fatal(err)
		}
		current := tmux.ResolvePane(name)
		if (current != previous) != tc.rotate {
			t.Fatalf("threshold %+v", tc)
		}
		previous = current
	}
	before := tmux.GetPaneOptionValue(previous, "@agent_role_entries")
	a.ResumePrompt = nil // A mention must neither rotate nor consume an entry.
	if err := dispatchBoardAction(a); err != nil {
		t.Fatal(err)
	}
	if tmux.ResolvePane(name) != previous || tmux.GetPaneOptionValue(previous, "@agent_role_entries") != before {
		t.Fatal("mention counted")
	}
}

func TestRunStatus(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	scope := tasks.Scope(dir)
	if err := tasks.SaveBoardConfig(dir, []byte("statuses: [{id: todo}, {id: test, run: echo, pass: done, fail: todo}, {id: done}]")); err != nil {
		t.Fatal(err)
	}
	for _, script := range []string{"echo success", "echo failure >&2; exit 7"} {
		task, _ := tasks.Add(scope, "CI ticket", nil)
		if err := tasks.Move(scope, task.ID, "test", ""); err != nil {
			t.Fatal(err)
		}
		events, _ := tasks.ReadEvents(scope, 0)
		entry := events[len(events)-1]
		if err := runStatus(board.Action{Dir: dir, Event: entry, Run: script, Pass: "done", Fail: "todo"}); err != nil {
			t.Fatal(err)
		}
		got, _ := tasks.Get(scope, task.ID)
		want := "done"
		if strings.Contains(script, "exit") {
			want = "todo"
		}
		if got.Status != want || len(got.Comments) != 1 || got.Comments[0].Author != "ci" || !strings.Contains(got.Comments[0].Body, "CI exit code") {
			t.Fatal(got)
		}
	}
}

// Snapshot names and bytes, including hidden state, to enforce read-only validation.
func boardValidateSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			result[path] = "directory"
			return nil
		}
		b, err := os.ReadFile(path)
		result[path] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestBoardValidate(t *testing.T) {
	for _, tc := range []struct {
		name, local, global string
		directory           bool
		args                []string
		code                int
	}{
		{name: "local", local: "statuses: [{id: todo}, {id: done}]"},
		{name: "malformed", local: "[", code: exitFailure},
		{name: "unknown field", local: "unknown: true", code: exitFailure},
		{name: "invalid reference", local: "statuses: [{id: todo, next: [missing]}, {id: done}]", code: exitFailure},
		{name: "multiple documents", local: "{}\n---\n{}", code: exitFailure},
		{name: "global invalid", global: "[", code: exitFailure},
		{name: "local overrides global", local: "{}", global: "["},
		{name: "global valid", global: "{}"},
		{name: "defaults"},
		{name: "directory read", directory: true, code: exitFailure},
		{name: "extra argument", args: []string{"extra"}, code: exitUsage},
		{name: "bad flag", args: []string{"--bad"}, code: exitUsage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "repo")
			config := filepath.Join(root, "config")
			t.Setenv("HOME", root)
			t.Setenv("XDG_CONFIG_HOME", config)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			for path, content := range map[string]string{filepath.Join(dir, ".tasks", "board.yaml"): tc.local, filepath.Join(config, "tagent", "board.yaml"): tc.global} {
				if content == "" {
					continue
				}
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.directory {
				if err := os.MkdirAll(filepath.Join(dir, ".tasks", "board.yaml"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			before := boardValidateSnapshot(t, root)
			var out, stderr bytes.Buffer
			code := cmdBoard(append([]string{"validate", "--dir", dir}, tc.args...), &out, &stderr)
			if code != tc.code {
				t.Fatalf("code %d, want %d: %s", code, tc.code, &stderr)
			}
			if code == exitOK {
				lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
				if lines[len(lines)-1] != "Board configuration valid." || stderr.Len() != 0 {
					t.Fatalf("stdout=%q stderr=%q", &out, &stderr)
				}
				for _, line := range lines[:len(lines)-1] {
					if !strings.HasPrefix(line, "warning: ") {
						t.Fatalf("unexpected output line %q", line)
					}
				}
			} else if out.Len() != 0 || (code == exitFailure && !strings.HasPrefix(stderr.String(), "board:")) {
				t.Fatalf("stdout=%q stderr=%q", &out, &stderr)
			}
			if !reflect.DeepEqual(before, boardValidateSnapshot(t, root)) {
				t.Fatal("validation changed files")
			}
		})
	}
}

func TestBoardValidateWorktree(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	main := filepath.Join(root, "main")
	linked := filepath.Join(root, "linked")
	for _, args := range [][]string{{"init", main}, {"-C", main, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "initial"}, {"-C", main, "worktree", "add", "-b", "linked", linked}} {
		if b, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, b)
		}
	}
	path := filepath.Join(main, ".tasks", "board.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("unknown: true"), 0600); err != nil {
		t.Fatal(err)
	}
	before := boardValidateSnapshot(t, root)
	var out, stderr bytes.Buffer
	if code := cmdBoard([]string{"validate", "--dir", linked}, &out, &stderr); code != exitFailure || out.Len() != 0 || !strings.HasPrefix(stderr.String(), "board:") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, &out, &stderr)
	}
	if !reflect.DeepEqual(before, boardValidateSnapshot(t, root)) {
		t.Fatal("validation changed files")
	}
}

// Role instructions are injected for every harness; only a legacy native file in
// the harness's own directory is loaded by the harness itself.
func TestRoleInstructionsPerHarness(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))
	if err := os.MkdirAll(filepath.Join(dir, ".tasks"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tasks", "board.yaml"), []byte("roles:\n  pirole: {agent: pi}\n  clauderole: {agent: claude}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(roles.LocalDir(dir), 0755)
	if err := os.WriteFile(filepath.Join(roles.LocalDir(dir), "shared.md"), []byte("Shared instructions."), 0644); err != nil {
		t.Fatal(err)
	}
	for _, harness := range []string{"pi", "claude", "codex", "opencode"} {
		if got := roleInstructions(dir, "shared", harness, true); got != "Shared instructions." {
			t.Fatalf("%s: %q", harness, got)
		}
	}
	if got := roleInstructions(dir, "nobody", "pi", true); got != "" {
		t.Fatalf("unknown role: %q", got)
	}
	legacy := filepath.Join(roles.DirFor(dir, "claude"), "native.md")
	os.MkdirAll(filepath.Dir(legacy), 0755)
	if err := os.WriteFile(legacy, []byte("Native."), 0644); err != nil {
		t.Fatal(err)
	}
	if got := roleInstructions(dir, "native", "claude", true); got != "" {
		t.Fatalf("native file sent twice: %q", got)
	}
	if got := roleInstructions(dir, "native", "claude", false); got != "Native." {
		t.Fatalf("board entry starts the harness directly, so the file is not loaded: %q", got)
	}
	if got := roleInstructions(dir, "native", "pi", true); got != "Native." {
		t.Fatalf("legacy file on another harness: %q", got)
	}
	if got := roleHarness(dir, "pirole"); got != "pi" {
		t.Fatalf("board harness: %q", got)
	}
	prompt := withRoleInstructions("shared", "Shared instructions.", "Ticket 1")
	if !strings.HasPrefix(prompt, "Your role is shared.") || !strings.Contains(prompt, "Shared instructions.") || !strings.HasSuffix(prompt, "Ticket 1") {
		t.Fatalf("prompt: %q", prompt)
	}
}

func TestRoleMemoryInDispatchPrompt(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0755)
	exec.Command("git", "-C", repo, "init", "-q").Run()
	if got := roles.Memory(repo, "shared"); got != "" {
		t.Fatalf("missing memory: %q", got)
	}
	os.MkdirAll(filepath.Dir(roles.MemoryPath(repo, "shared")), 0755)
	if err := os.WriteFile(roles.MemoryPath(repo, "shared"), []byte("Remember the scope."), 0644); err != nil {
		t.Fatal(err)
	}
	prompt := withRoleMemory("shared", roles.Memory(repo, "shared"), "Ticket 2")
	if !strings.Contains(prompt, "Remember the scope.") || !strings.HasSuffix(prompt, "Ticket 2") {
		t.Fatalf("prompt: %q", prompt)
	}
}
