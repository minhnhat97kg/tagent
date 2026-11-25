package main

import (
	"strings"
	"testing"

	"tagent/internal/tmux"
)

func TestHaikuPaneGetsCompactWindow(t *testing.T) {
	for _, model := range []string{"haiku", "sonnet"} {
		t.Run(model, func(t *testing.T) {
			t.Setenv("TMUX_PANE", "%1")
			fake := tmux.NewFake()
			fake.Set("$1", "display-message", "-t", "%1", "-p", "#{session_id}")
			restore := tmux.SetRunner(fake)
			defer restore()
			cwd := t.TempDir()
			fakeProjectSession(fake, cwd)
			spawnAgent(spawnOpts{Agent: "claude", Model: model, Cwd: cwd})
			var command string
			for _, call := range fake.Calls() {
				if len(call) > 0 && call[0] == "new-window" {
					command = call[len(call)-1]
				}
			}
			hasEnv := strings.HasPrefix(command, "env CLAUDE_CODE_AUTO_COMPACT_WINDOW=100000 ")
			if hasEnv != (model == "haiku") {
				t.Fatalf("%s: command %q", model, command)
			}
		})
	}
}
