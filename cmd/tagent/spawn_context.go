package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
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
	if o.Remote.Host != "" {
		var err error
		if rc, err = remoteRoleContext(rc); err != nil {
			return nil, err
		}
	}
	mcpFile := ""
	if rc.MCP != nil && len(*rc.MCP) > 0 {
		b, err := mcpConfig(o.ConfigDir, *rc.MCP, o.BrowserProfile)
		if err != nil {
			return nil, err
		}
		// Written under the board dir, never on the command line: a server entry may hold a token.
		dir := tasks.BoardDir(tasks.Scope(o.ConfigDir))
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		file := "mcp-" + strings.ReplaceAll(name, "/", "-")
		if o.BrowserProfile != "" {
			// The profile is per ticket, so the file is too: two tickets on one role must not share it.
			file += "-" + filepath.Base(filepath.Dir(o.BrowserProfile))
		}
		mcpFile = filepath.Join(dir, file+".json")
		if err := os.WriteFile(mcpFile, b, 0600); err != nil {
			return nil, err
		}
	}
	return rc.Args(harness, mcpFile)
}

// remoteRoleContext drops what names the local MCP servers, which a remote host
// cannot reach: no MCP config, and mcp__ tool names. A tool list that is left
// empty would widen to the harness default, so that is an error instead.
func remoteRoleContext(rc tasks.RoleContext) (tasks.RoleContext, error) {
	keep := func(list []string) []string {
		var out []string
		for _, s := range list {
			if !strings.HasPrefix(s, "mcp__") {
				out = append(out, s)
			}
		}
		return out
	}
	if len(rc.Tools) > 0 && len(keep(rc.Tools)) == 0 {
		return rc, fmt.Errorf("remote role tools name only MCP tools, which a remote host does not have")
	}
	rc.Tools, rc.Allow = keep(rc.Tools), keep(rc.Allow)
	if rc.MCP != nil {
		rc.MCP = new([]string)
	}
	return rc, nil
}

// browserServer is the MCP server a browser ticket points at its Chrome profile.
// Its CLI takes --user-data-dir; no other server's args change.
const browserServer = "playwright"

// mcpConfig builds an MCP config holding only the named servers. "tagent" is
// generated; any other name is copied from the project or the Claude config.
// A non-empty profile (browser ticket) is passed to browserServer as --user-data-dir.
func mcpConfig(dir string, names []string, profile string) ([]byte, error) {
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
		if name == browserServer && profile != "" {
			s = withUserDataDir(s, profile)
		}
		servers[name] = s
	}
	return json.Marshal(map[string]any{"mcpServers": servers})
}

// withUserDataDir returns a copy of the server entry with --user-data-dir=dir
// appended to its args. The stored entry is not changed.
func withUserDataDir(s any, dir string) any {
	m, ok := s.(map[string]any)
	if !ok {
		return s
	}
	args, _ := m["args"].([]any)
	c := maps.Clone(m)
	c["args"] = append(slices.Clone(args), "--user-data-dir="+dir)
	return c
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
