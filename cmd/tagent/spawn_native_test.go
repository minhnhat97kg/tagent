package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/roles"
	"tagent/internal/setup"
	"tagent/internal/tmux"
)

func TestWorkflowNativeRoleLaunchUsesLiveConfig(t *testing.T) {
	for _, harness := range []string{"claude", "opencode"} {
		t.Run(harness, func(t *testing.T) {
			main, tree := t.TempDir(), t.TempDir()
			t.Setenv("TMUX_PANE", "%1")
			t.Setenv("OPENCODE_CONFIG_CONTENT", `{"theme":"test","agent":{"other":{"prompt":"keep"}}}`)
			for _, dir := range []string{main, tree} {
				if err := os.MkdirAll(roles.DirFor(dir, harness), 0700); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(roles.DirFor(main, harness), "reviewer.md")
			for _, prompt := range []string{"main only", "edited main"} {
				definition := "---\ndescription: Live reviewer\nmodel: test-model\n---\n" + prompt
				if err := os.WriteFile(path, []byte(definition), 0600); err != nil {
					t.Fatal(err)
				}
				if prompt == "edited main" {
					stale := filepath.Join(roles.DirFor(tree, harness), "reviewer.md")
					if err := os.WriteFile(stale, []byte("stale tree definition"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				role, ok := roles.GetIn(main, "reviewer")
				if !ok {
					t.Fatal("missing main role")
				}
				config, err := nativeRoleConfig(role)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(config, prompt) || strings.Contains(config, "stale") || !strings.Contains(config, `"model":"test-model"`) {
					t.Fatal(config)
				}
				if harness == "opencode" && !strings.Contains(config, `"other":{"prompt":"keep"}`) {
					t.Fatal(config)
				}
				fake := tmux.NewFake()
				fake.Set("$1", "display-message", "-t", "%1", "-p", "#{session_id}")
				restore := tmux.SetRunner(fake)
				// Undeclared pane creation fails, but records the exact harness invocation.
				effort := ""
				if harness == "claude" {
					effort = "high"
				}
				_, _, _ = spawnAgent(spawnOpts{Agent: harness, Role: "reviewer", ConfigDir: main, Cwd: tree, Effort: effort})
				restore()
				found := false
				for _, call := range fake.Calls() {
					if len(call) == 0 || call[0] != "new-window" {
						continue
					}
					command := call[len(call)-1]
					flag := "--agents " + setup.ShellQuote(config)
					if harness == "opencode" {
						flag = "OPENCODE_CONFIG_CONTENT=" + setup.ShellQuote(config)
					}
					if effort != "" && !strings.Contains(command, "--effort high") {
						t.Fatal(command)
					}
					if !strings.Contains(command, flag) {
						t.Fatalf("missing live config: %s", command)
					}
					found = true
				}
				if !found {
					t.Fatal("harness launch not reached")
				}
			}
		})
	}
}
