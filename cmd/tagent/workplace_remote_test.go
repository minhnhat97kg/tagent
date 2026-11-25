package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"tagent/internal/board"
	"tagent/internal/scrape"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"tagent/internal/workplace"
)

func readRemoteLog(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

// A `workplace: remote` ticket starts its harness over ssh (a fake ssh on PATH
// here). Its status comes from the scraper and survives its ticks, nothing runs
// git on the remote path, and done only records where the files are.
func TestWorkplaceRemoteRunsOverSSH(t *testing.T) {
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	argsLog, inLog, gitLog := filepath.Join(t.TempDir(), "args"), filepath.Join(t.TempDir(), "in"), filepath.Join(t.TempDir(), "git")
	// The fake ssh prints ready, then a working line after each line it reads.
	ssh := "#!/bin/sh\necho \"$*\" >> " + argsLog + "\necho ready\nwhile IFS= read -r line; do printf '%s\\n' \"$line\" >> " + inLog + "; echo 'esc to interrupt'; done\n"
	os.WriteFile(filepath.Join(bin, "ssh"), []byte(ssh), 0700)
	os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\necho \"$PWD $*\" >> "+gitLog+"\nexec "+realGit+" \"$@\"\n"), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	dir, scope, ticket := startWorkplaceTicket(t, "workplace: remote\n")
	tmux.RunTmux("set-option", "-g", "default-shell", "/bin/sh") // no rc files reorder the fake PATH
	os.WriteFile(filepath.Join(dir, ".tasks", "board.yaml"), []byte("remote: {host: fakehost, dir: /nonexistent/remote}\nroles:\n  planner: {agent: claude, mcp: [tagent]}\n"), 0600)

	// The scraper runs in-process. Its pid is set first so dispatch never spawns a second one.
	tmux.SetOption("@agent_scrape_pid", strconv.Itoa(os.Getpid()))
	done := make(chan struct{})
	go func() { scrape.Run(); close(done) }()
	t.Cleanup(func() {
		tmux.SetOption("@agent_scrape_pid", "0") // the scraper exits on its next tick
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("scraper did not stop")
		}
	})

	a := board.Action{Dir: dir, Agent: "planner", Prompt: "first prompt"}
	a.Event.Task = ticket.ID
	if err := dispatchBoardAction(a); err != nil {
		t.Fatal(err)
	}
	name := "planner-" + ticket.ID[max(0, len(ticket.ID)-4):]
	pane := tmux.ResolvePane(name)
	if pane == name {
		t.Fatal("remote pane not started")
	}
	args := readRemoteLog(argsLog)
	for _, want := range []string{"-o BatchMode=yes", "-o ServerAliveCountMax=3", "fakehost", "cd /nonexistent/remote && exec "} {
		if !strings.Contains(args, want) {
			t.Fatalf("ssh args %q lack %q", args, want)
		}
	}
	if strings.Contains(args, "--mcp-config") {
		t.Fatalf("remote agent got an MCP config: %q", args)
	}

	deadline := time.Now().Add(10 * time.Second)
	for tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt) != "running" && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(readRemoteLog(inLog), "Remote workplace") {
		t.Fatalf("prompt not sent over ssh: %q", readRemoteLog(inLog))
	}
	time.Sleep(3500 * time.Millisecond) // past the scraper's ticks
	if tmux.ResolvePane(name) != pane {
		t.Fatal("remote pane closed while running")
	}
	if st := tmux.GetPaneOptionValue(pane, tmux.PaneStatusOpt); st != "running" {
		t.Fatalf("status %q after scrape ticks, want running", st)
	}
	if src := tmux.GetPaneOptionValue(pane, tmux.PaneAgentSrc); src != tmux.AgentSrcRemote {
		t.Fatalf("pane src %q, want remote", src)
	}
	if agent := tmux.GetPaneOptionValue(pane, tmux.PaneAgent); agent != "claude" {
		t.Fatalf("pane agent %q, want claude", agent)
	}
	if refs, _ := git(dir, "for-each-ref", "refs/tagent"); refs != "" {
		t.Fatalf("remote pane made checkpoints: %q", refs)
	}

	if err := tasks.Move(scope, ticket.ID, "done", ""); err != nil {
		t.Fatal(err)
	}
	if tmux.ResolvePane(name) != name {
		t.Fatal("remote pane kept after done")
	}
	events, _ := tasks.ReadEvents(scope, 0)
	found := false
	for _, e := range events {
		found = found || strings.Contains(e.Message, "remote files stay in fakehost:/nonexistent/remote")
	}
	if !found {
		t.Fatalf("done did not record where the files are: %+v", events)
	}
	if strings.Contains(readRemoteLog(gitLog), "/nonexistent/remote") {
		t.Fatalf("git ran on the remote path:\n%s", readRemoteLog(gitLog))
	}
	if out, _ := git(dir, "branch", "--list", "agent/workflow-*"); out != "" {
		t.Fatalf("workflow branches: %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, workplace.Dir)); !os.IsNotExist(err) {
		t.Fatalf(".worktrees: %v", err)
	}
}

func TestRemoteRoleContextDropsMCPNames(t *testing.T) {
	rc, err := remoteRoleContext(tasks.ScoutContext())
	if err != nil || len(*rc.MCP) != 0 || !slices.Equal(rc.Tools, []string{"Read", "Grep", "Glob"}) || len(rc.Allow) != 0 {
		t.Fatalf("scout on remote: %+v %v", rc, err)
	}
	if _, err := remoteRoleContext(tasks.RoleContext{Tools: []string{"mcp__tagent__task_show"}}); err == nil {
		t.Fatal("tool list of only MCP names widened instead of failing")
	}
}
