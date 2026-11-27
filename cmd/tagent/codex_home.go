package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"tagent/internal/library"
	"tagent/internal/tasks"
)

// codexBase is the user's codex directory: the harness entry's CODEX_HOME, the
// environment's, else ~/.codex.
func codexBase(env []string) string {
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "CODEX_HOME="); ok && v != "" {
			return v
		}
	}
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

// codexRoleHome builds the CODEX_HOME of one launch of a role that selects skills
// or MCP servers, and returns "" for any other role. It holds the user's
// config.toml (without its MCP servers when the role selects them), the selected
// skills and servers, and a link to the user's login. Nothing in base is written.
// ponytail: one directory per launch, never removed; prune if the board dir grows.
func codexRoleHome(o spawnOpts, boardRole, base string) (string, error) {
	rc, name, err := roleContext(o, boardRole)
	if err != nil || rc.Skills == nil && rc.MCP == nil {
		return "", err
	}
	parent := filepath.Join(tasks.BoardDir(tasks.Scope(o.ConfigDir)), "codex-"+strings.ReplaceAll(name, "/", "-"))
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	home, err := os.MkdirTemp(parent, "launch-")
	if err != nil {
		return "", err
	}
	if err := codexSkills(o, rc.Skills, base, filepath.Join(home, "skills")); err != nil {
		return "", err
	}

	config := map[string]any{}
	if _, err := toml.DecodeFile(filepath.Join(base, "config.toml"), &config); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read codex config: %w", err)
	}
	if rc.MCP != nil {
		delete(config, "mcp_servers")
		if len(*rc.MCP) > 0 {
			servers, err := codexServers(o, *rc.MCP)
			if err != nil {
				return "", err
			}
			config["mcp_servers"] = servers
		}
	}
	var out bytes.Buffer
	if err := toml.NewEncoder(&out).Encode(config); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), out.Bytes(), 0o600); err != nil {
		return "", err
	}

	auth := filepath.Join(base, "auth.json")
	if _, err := os.Stat(auth); err != nil {
		fmt.Fprintf(os.Stderr, "warning: no %s: codex may start logged out\n", auth)
		return home, nil
	}
	return home, os.Symlink(auth, filepath.Join(home, "auth.json"))
}

// codexSkills fills dir with links: the selected library skills, or each of the
// user's own skills when the role does not select any.
func codexSkills(o spawnOpts, selected *[]string, base, dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	links := map[string]string{}
	if selected == nil {
		// One link per skill: a link to the whole folder would let codex write into base.
		entries, err := os.ReadDir(filepath.Join(base, "skills"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				links[e.Name()] = filepath.Join(base, "skills", e.Name())
			}
		}
	} else if len(*selected) > 0 {
		lib, err := library.Load(tasks.MainRoot(o.ConfigDir))
		if err != nil {
			return err
		}
		for _, name := range *selected {
			entry, ok := lib.Skills[name]
			if !ok {
				return fmt.Errorf("skill %q is not in the library", name)
			}
			links[entry.Name] = entry.Path
		}
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

// codexServers resolves names like any other launch (approval included) and
// rewrites them for config.toml. A "${VAR}" reference is passed by name through
// env_vars, so tagent never writes its value.
func codexServers(o spawnOpts, names []string) (map[string]any, error) {
	b, err := mcpConfig(o.ConfigDir, names, o.BrowserProfile)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for name, raw := range doc.Servers {
		var s struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			URL     string            `json:"url"`
			Env     map[string]string `json:"env"`
		}
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		entry := map[string]any{}
		if s.URL != "" {
			entry["url"] = s.URL
		} else {
			entry["command"] = s.Command
			if len(s.Args) > 0 {
				entry["args"] = s.Args
			}
		}
		env, vars := map[string]string{}, []string{}
		for key, value := range s.Env {
			ref, isRef := strings.CutPrefix(value, "${")
			switch {
			case !isRef:
				env[key] = value
			case ref == key+"}":
				vars = append(vars, key)
			default:
				return nil, fmt.Errorf("MCP server %q: codex passes a variable only under its own name, so %s cannot come from %s", name, key, value)
			}
		}
		if len(env) > 0 {
			entry["env"] = env
		}
		if len(vars) > 0 {
			slices.Sort(vars)
			entry["env_vars"] = vars
		}
		out[name] = entry
	}
	return out, nil
}
