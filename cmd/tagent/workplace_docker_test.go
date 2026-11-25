package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"tagent/internal/board"
	"tagent/internal/scrape"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

// runDockerTicket dispatches one ticket of a `workplace: docker` recipe with a fake
// docker on PATH, waits for the pane's docker exec, then moves the ticket to done.
// It returns the fake docker's calls and the ticket's history messages.
func runDockerTicket(t *testing.T, workplaceLine string) (calls string, messages []string, id string) {
	t.Helper()
	bin := t.TempDir()
	callsLog, inLog := filepath.Join(t.TempDir(), "calls"), filepath.Join(t.TempDir(), "in")
	exists := filepath.Join(t.TempDir(), "exists") // the container: created by `create`, removed by `rm`
	// `start` fails until the container exists. exec behaves like a harness: ready, then a working line per input line.
	stub := "#!/bin/sh\necho \"$*\" >> " + callsLog + "\ncase \"$1\" in\n" +
		"start) [ -e " + exists + " ] || exit 1 ;;\n" +
		"create) touch " + exists + " ;;\n" +
		"rm) rm -f " + exists + " ;;\n" +
		"exec) echo ready; while IFS= read -r line; do printf '%s\\n' \"$line\" >> " + inLog + "; echo 'esc to interrupt'; done ;;\n" +
		"esac\n"
	os.WriteFile(filepath.Join(bin, "docker"), []byte(stub), 0700)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	dir, scope, ticket := startWorkplaceTicket(t, workplaceLine)
	tmux.RunTmux("set-option", "-g", "default-shell", "/bin/sh") // no rc files reorder the fake PATH
	os.WriteFile(filepath.Join(dir, ".tasks", "board.yaml"), []byte("docker: {image: devbox:latest}\nroles:\n  planner: {agent: claude, mcp: [tagent]}\n"), 0600)

	// The scraper runs in-process; its pid is set first so dispatch never spawns a second one.
	tmux.SetOption("@agent_scrape_pid", strconv.Itoa(os.Getpid()))
	done := make(chan struct{})
	go func() { scrape.Run(); close(done) }()
	t.Cleanup(func() {
		tmux.SetOption("@agent_scrape_pid", "0")
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
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(readRemoteLog(callsLog), "exec -it") && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if err := tasks.Move(scope, ticket.ID, "done", ""); err != nil {
		t.Fatal(err)
	}
	events, _ := tasks.ReadEvents(scope, 0)
	for _, e := range events {
		messages = append(messages, e.Message)
	}
	return readRemoteLog(callsLog), messages, ticket.ID
}

// A `workplace: docker` ticket runs its harness in a container: create, then exec
// into it, then rm on done, in that order, with the MCP config left out.
func TestWorkplaceDockerCreateExecRm(t *testing.T) {
	calls, _, id := runDockerTicket(t, "workplace: docker\n")
	lines := strings.Split(strings.TrimSpace(calls), "\n")
	at := func(prefix string) int {
		for i, line := range lines {
			if strings.HasPrefix(line, prefix) {
				return i
			}
		}
		return -1
	}
	c := at("create --name tagent-" + id + " ")
	e := at("exec -it -w /work tagent-" + id + " ")
	r := at("rm -f tagent-" + id)
	if c < 0 || e <= c || r <= e {
		t.Fatalf("docker calls missing or out of order:\n%s", calls)
	}
	if strings.Contains(calls, "--mcp-config") {
		t.Fatalf("docker agent got an MCP config:\n%s", calls)
	}
}

// With `cleanup: keep` the container stays after done, and the ticket says so.
func TestWorkplaceDockerKeepsContainer(t *testing.T) {
	calls, messages, id := runDockerTicket(t, "workplace: docker\ncleanup: keep\n")
	if strings.Contains(calls, "rm -f") {
		t.Fatalf("kept container was removed:\n%s", calls)
	}
	want := "cleanup: kept container tagent-" + id
	for _, m := range messages {
		if strings.Contains(m, want) {
			return
		}
	}
	t.Fatalf("done did not record %q: %v", want, messages)
}
