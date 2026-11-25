package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tagent/internal/roles"
	"tagent/internal/tasks"
)

// roleArgs turns the role's context from board.yaml (or the scout preset) into
// harness flags. It runs before the pane exists, so a bad config never starts an
// agent with less restriction than it asked for.
func roleArgs(o spawnOpts, boardRole string) ([]string, error) {
	harness := roles.Harness(o.Agent)
	var rc tasks.RoleContext
	name := ""
	if boardRole != "" {
		cfg, err := tasks.LoadBoardConfig(o.ConfigDir)
		if err != nil {
			return nil, err
		}
		if br, ok := cfg.Roles[boardRole]; ok && roles.Harness(br.Agent) == harness {
			rc, name = br.RoleContext, boardRole
		}
	}
	if o.Preset == "scout" && isClaude(o.Agent) {
		rc, name = tasks.ScoutContext(), "scout"
	}
	if !rc.Set() {
		return nil, nil
	}
	mcpFile := ""
	if rc.MCP != nil && len(*rc.MCP) > 0 {
		b, err := mcpConfig(o.ConfigDir, *rc.MCP)
		if err != nil {
			return nil, err
		}
		// Written under the board dir, never on the command line: a server entry may hold a token.
		dir := tasks.BoardDir(tasks.Scope(o.ConfigDir))
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		mcpFile = filepath.Join(dir, "mcp-"+strings.ReplaceAll(name, "/", "-")+".json")
		if err := os.WriteFile(mcpFile, b, 0600); err != nil {
			return nil, err
		}
	}
	return rc.Args(harness, mcpFile)
}

// mcpConfig builds an MCP config holding only the named servers. "tagent" is
// generated; any other name is copied from the project or the Claude config.
func mcpConfig(dir string, names []string) ([]byte, error) {
	servers := map[string]any{}
	for _, name := range names {
		if name == "tagent" {
			exe, err := os.Executable()
			if err != nil {
				return nil, err
			}
			servers[name] = map[string]any{"command": exe, "args": []string{"mcp"}}
			continue
		}
		s, ok := findMCPServer(tasks.MainRoot(dir), name)
		if !ok {
			return nil, fmt.Errorf("unknown MCP server %q (not in .mcp.json or ~/.claude.json)", name)
		}
		servers[name] = s
	}
	return json.Marshal(map[string]any{"mcpServers": servers})
}

// findMCPServer looks in the project's .mcp.json, then in ~/.claude.json for the
// project's servers and then the user's top-level servers.
func findMCPServer(root, name string) (any, bool) {
	home, _ := os.UserHomeDir()
	type servers struct {
		MCPServers map[string]any `json:"mcpServers"`
	}
	for _, file := range []string{filepath.Join(root, ".mcp.json"), filepath.Join(home, ".claude.json")} {
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var doc struct {
			servers
			Projects map[string]servers `json:"projects"`
		}
		if json.Unmarshal(b, &doc) != nil {
			continue
		}
		if s, ok := doc.Projects[root].MCPServers[name]; ok {
			return s, true
		}
		if s, ok := doc.MCPServers[name]; ok {
			return s, true
		}
	}
	return nil, false
}
