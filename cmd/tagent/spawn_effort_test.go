package main

import (
	"strings"
	"tagent/internal/setup"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
	"testing"
)

func TestEffortLaunch(t *testing.T) {
	for _, harness := range []string{"pi", "claude", "codex", "opencode", "unknown"} {
		for _, effort := range []string{"", "high", "bogus", "high; echo injected"} {
			t.Run(harness+"/"+effort, func(t *testing.T) {
				t.Setenv("TMUX_PANE", "%1")
				fake := tmux.NewFake()
				fake.Set("$1", "display-message", "-t", "%1", "-p", "#{session_id}")
				restore := tmux.SetRunner(fake)
				defer restore()
				cwd := t.TempDir()
				fakeProjectSession(fake, cwd)
				_, _, err := spawnAgent(spawnOpts{Agent: harness, Model: "model:high", Effort: effort, Cwd: cwd, Worktree: false})
				valid := effort == "" || (effort == "high" && harness != "opencode" && harness != "unknown")
				found := false
				for _, call := range fake.Calls() {
					if len(call) == 0 || call[0] != "new-window" {
						continue
					}
					found = true
					command := call[len(call)-1]
					want := setup.ShellQuote(harness) + " --model model:high"
					if effort != "" {
						switch harness {
						case "pi":
							want += " --thinking high"
						case "claude":
							want += " --effort high"
						case "codex":
							want += " -c " + setup.ShellQuote(`model_reasoning_effort="high"`)
						}
					}
					if command != want {
						t.Fatalf("command %q want %q", command, want)
					}
				}
				if found != valid {
					t.Fatalf("launch=%v valid=%v err=%v", found, valid, err)
				}
				if !valid && (err == nil || !strings.Contains(err.Error(), "effort")) {
					t.Fatal(err)
				}
				if !valid {
					for _, call := range fake.Calls() {
						if len(call) > 0 && (call[0] == "new-session" || call[0] == "new-window") {
							t.Fatal(call)
						}
					}
				}
			})
		}
	}
}

// fakeProjectSession declares the session creation spawnAgent does for cwd.
func fakeProjectSession(f *tmux.Fake, cwd string) {
	f.Set("$1 @1", "new-session", "-d", "-s", tmux.SessionName(tasks.MainRoot(cwd)), "-c", tasks.MainRoot(cwd), "-P", "-F", "#{session_id} #{window_id}")
}
