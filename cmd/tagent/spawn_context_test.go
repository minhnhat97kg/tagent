package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"tagent/internal/tmux"
)

func TestSpawnAppliesBoardRoleContext(t *testing.T) {
	t.Setenv("TMUX_PANE", "%1")
	cwd := t.TempDir()
	os.MkdirAll(filepath.Join(cwd, ".tasks"), 0700)
	board := "roles:\n  reviewer: {agent: claude, mcp: [tagent], tools: [Read], skills: []}\n"
	os.WriteFile(filepath.Join(cwd, ".tasks", "board.yaml"), []byte(board), 0600)
	fake := tmux.NewFake()
	fake.Set("$1", "display-message", "-t", "%1", "-p", "#{session_id}")
	restore := tmux.SetRunner(fake)
	defer restore()
	fakeProjectSession(fake, cwd)
	// The fake records the new-window call and then reports it undeclared; only the command matters here.
	spawnAgent(spawnOpts{Agent: "claude", BoardRole: "reviewer", Cwd: cwd, ConfigDir: cwd})
	var command string
	for _, call := range fake.Calls() {
		if len(call) > 0 && call[0] == "new-window" {
			command = call[len(call)-1]
		}
	}
	for _, want := range []string{"--disable-slash-commands", "--strict-mcp-config", "--mcp-config=", "--tools=Read"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command %q lacks %q", command, want)
		}
	}
}

func TestSpawnFailsOnUnknownMCPServer(t *testing.T) {
	t.Setenv("TMUX_PANE", "%1")
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	os.MkdirAll(filepath.Join(cwd, ".tasks"), 0700)
	os.WriteFile(filepath.Join(cwd, ".tasks", "board.yaml"), []byte("roles:\n  reviewer: {agent: claude, mcp: [nope]}\n"), 0600)
	fake := tmux.NewFake()
	fake.Set("$1", "display-message", "-t", "%1", "-p", "#{session_id}")
	restore := tmux.SetRunner(fake)
	defer restore()
	fakeProjectSession(fake, cwd)
	_, _, err := spawnAgent(spawnOpts{Agent: "claude", BoardRole: "reviewer", Cwd: cwd, ConfigDir: cwd})
	if err == nil || !strings.Contains(err.Error(), `unknown MCP server "nope"`) {
		t.Fatalf("err=%v", err)
	}
	for _, call := range fake.Calls() {
		if len(call) > 0 && call[0] == "new-window" {
			t.Fatal("pane started despite bad MCP config")
		}
	}
}

func TestMCPConfigPassesBrowserProfileOnlyToBrowserServer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := t.TempDir()
	project := `{"mcpServers":{"playwright":{"command":"npx","args":["@playwright/mcp"]},"fs":{"command":"fs-mcp","args":["--root","/x"]}}}`
	os.WriteFile(filepath.Join(cwd, ".mcp.json"), []byte(project), 0600)
	profile := "/ctx/browser/T1/profile"
	type doc struct {
		MCPServers map[string]struct {
			Args []string `json:"args"`
		} `json:"mcpServers"`
	}
	decode := func(profile string) doc {
		b, err := mcpConfig(cwd, []string{"playwright", "fs"}, profile)
		if err != nil {
			t.Fatal(err)
		}
		var d doc
		if err := json.Unmarshal(b, &d); err != nil {
			t.Fatal(err)
		}
		return d
	}
	got := decode(profile)
	if want := []string{"@playwright/mcp", "--user-data-dir=" + profile}; !slices.Equal(got.MCPServers["playwright"].Args, want) {
		t.Fatalf("browser args %q, want %q", got.MCPServers["playwright"].Args, want)
	}
	if want := []string{"--root", "/x"}; !slices.Equal(got.MCPServers["fs"].Args, want) {
		t.Fatalf("other server args changed: %q", got.MCPServers["fs"].Args)
	}
	plain := decode("")
	if want := []string{"@playwright/mcp"}; !slices.Equal(plain.MCPServers["playwright"].Args, want) {
		t.Fatalf("non-browser spawn args %q, want %q", plain.MCPServers["playwright"].Args, want)
	}
}
