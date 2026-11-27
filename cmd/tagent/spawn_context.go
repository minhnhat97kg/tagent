package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"tagent/internal/library"
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
	if o.Remote.Host != "" || o.Container != "" { // neither has tagent's MCP servers
		var err error
		if rc, err = remoteRoleContext(rc); err != nil {
			return nil, err
		}
	}
	var skillArgs []string
	if rc.Skills != nil && len(*rc.Skills) > 0 {
		lib, err := library.Load(tasks.MainRoot(o.ConfigDir))
		if err != nil {
			return nil, err
		}
		selected := make([]library.Skill, 0, len(*rc.Skills))
		for _, skill := range *rc.Skills {
			entry, ok := lib.Skills[skill]
			if !ok {
				return nil, fmt.Errorf("skill %q is not in the library", skill)
			}
			selected = append(selected, entry)
		}
		if harness == "pi" {
			skillArgs = append(skillArgs, "--no-skills")
			for _, entry := range selected {
				skillArgs = append(skillArgs, "--skill", entry.Path)
			}
		} else if harness == "claude" {
			dir := filepath.Join(tasks.BoardDir(tasks.Scope(o.ConfigDir)), "lib-"+name)
			if err := os.MkdirAll(dir, 0700); err != nil {
				return nil, err
			}
			plugin, err := os.MkdirTemp(dir, "launch-")
			if err != nil {
				return nil, err
			}
			plugin = filepath.Join(plugin, "tlib")
			if err := os.MkdirAll(filepath.Join(plugin, "skills"), 0700); err != nil {
				return nil, err
			}
			for _, entry := range selected {
				if err := os.Symlink(entry.Path, filepath.Join(plugin, "skills", entry.Name)); err != nil && !os.IsExist(err) {
					return nil, err
				}
			}
			manifest := []byte("{\"name\":\"tlib\",\"description\":\"tagent library skills\",\"version\":\"1.0.0\"}\n")
			if err := os.MkdirAll(filepath.Join(plugin, ".claude-plugin"), 0700); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(plugin, ".claude-plugin", "plugin.json"), manifest, 0600); err != nil {
				return nil, err
			}
			skillArgs = append(skillArgs, "--plugin-dir", plugin)
		} else {
			return nil, fmt.Errorf("skill list is not supported for %s", harness)
		}
		rc.Skills = nil
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
	args, err := rc.Args(harness, mcpFile)
	return append(args, skillArgs...), err
}

// noteUnappliedLibrary tells the user, on stderr and on the ticket, that a remote
// or docker agent starts without the skills and MCP servers its role selects. A
// comment that cannot be written fails the launch, so the notice is never lost.
func noteUnappliedLibrary(o spawnOpts, role string) error {
	place := ""
	switch {
	case o.Container != "":
		place = "docker"
	case o.Remote.Host != "":
		place = "remote"
	default:
		return nil
	}
	cfg, err := tasks.LoadBoardConfig(o.ConfigDir)
	if err != nil {
		return err
	}
	var names []string
	for _, list := range []*[]string{cfg.Roles[role].Skills, cfg.Roles[role].MCP} {
		if list != nil {
			names = append(names, *list...)
		}
	}
	if len(names) == 0 {
		return nil
	}
	notice := "library: skills/mcp not applied on " + place + ": " + strings.Join(names, ", ")
	fmt.Fprintln(os.Stderr, "warning: "+notice)
	if o.Ticket != "" {
		if err := tasks.Comment(tasks.Scope(o.ConfigDir), o.Ticket, "tagent", notice, nil); err != nil {
			return fmt.Errorf("comment %s library notice on ticket %s: %w", place, o.Ticket, err)
		}
	}
	return nil
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
	if rc.Skills != nil && len(*rc.Skills) > 0 {
		rc.Skills = new([]string)
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
	lib, err := library.Load(tasks.MainRoot(dir))
	if err != nil {
		return nil, err
	}
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
		var s any
		if entry, found := lib.Servers[name]; found {
			state, err := os.UserConfigDir()
			if err != nil {
				return nil, err
			}
			approved, err := library.Approved(filepath.Join(state, "tagent"), entry)
			if err != nil {
				return nil, err
			}
			if !approved {
				return nil, fmt.Errorf("MCP server %q requires user approval before launch (command: %s)", name, entry.Command)
			}
			for key, ref := range entry.Env {
				variable := strings.TrimSuffix(strings.TrimPrefix(ref, "${"), "}")
				if _, ok := os.LookupEnv(variable); !ok {
					return nil, fmt.Errorf("MCP server %q requires environment variable %s", name, variable)
				}
				_ = key
			}
			s = map[string]any{
				"command": entry.Command,
				"args":    entry.Args,
				"env":     entry.Env,
			}
		} else {
			var ok bool
			s, ok = findMCPServer(tasks.MainRoot(dir), name)
			if !ok {
				return nil, fmt.Errorf("unknown MCP server %q (not in library, .mcp.json or ~/.claude.json)", name)
			}
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
