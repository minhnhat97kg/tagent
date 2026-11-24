package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

func gitInit(t *testing.T, dir string) string {
	t.Helper()
	os.MkdirAll(dir, 0o755)
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "i"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatal(string(out))
		}
	}
	return tasks.MainRoot(dir)
}

// spawnCalls spawns one agent per opts and returns the new-session names and
// the new-window (target, name) pairs it ran.
func spawnCalls(t *testing.T, opts ...spawnOpts) (sessions []string, windows [][2]string) {
	t.Helper()
	fake := tmux.NewFake()
	for _, o := range opts {
		fakeProjectSession(fake, o.Cwd)
	}
	defer tmux.SetRunner(fake)()
	for _, o := range opts {
		o.Agent = "pi"
		spawnAgent(o)
	}
	for _, c := range fake.Calls() {
		switch c[0] {
		case "new-session":
			// the fake never lists sessions back, so repeats are expected
			if len(sessions) == 0 || sessions[len(sessions)-1] != c[3] {
				sessions = append(sessions, c[3])
			}
		case "new-window":
			windows = append(windows, [2]string{c[3], c[5]})
		}
	}
	return
}

func TestSessionPerProject(t *testing.T) {
	root := t.TempDir()
	a := gitInit(t, filepath.Join(root, "ws", "a"))
	b := gitInit(t, filepath.Join(root, "ws", "b"))
	wt := filepath.Join(a, ".worktrees", "w1")
	if out, err := exec.Command("git", "-C", a, "worktree", "add", "-q", wt, "-b", "w1").CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	ws := filepath.Join(root, "ws")
	sessions, windows := spawnCalls(t,
		spawnOpts{Cwd: a, Name: "one"},
		spawnOpts{Cwd: a, Name: "two", Window: "dev#46"},
		spawnOpts{Cwd: wt, Name: "wt"},
		spawnOpts{Cwd: b, Name: "bee"},
		spawnOpts{Cwd: ws, Name: "top"},
	)
	want := []string{tmux.SessionName(a), tmux.SessionName(b), tmux.SessionName(ws)}
	if len(sessions) != 3 || sessions[0] != want[0] || sessions[1] != want[1] || sessions[2] != want[2] {
		t.Fatalf("sessions %v want %v", sessions, want)
	}
	// All fake sessions share id $1, so windows are checked by name only.
	names := []string{"one", "dev#46", "wt", "bee", "top"}
	if len(windows) != len(names) {
		t.Fatalf("windows %v", windows)
	}
	for i, n := range names {
		if windows[i][1] != n {
			t.Fatalf("window %d = %q want %q", i, windows[i][1], n)
		}
	}
}
