package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"

	"tagent/internal/library"

	"gopkg.in/yaml.v3"
)

var libraryName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var libraryEnvRef = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)

type libraryListing struct {
	Skills  []library.Skill   `json:"skills"`
	Servers []library.Server  `json:"servers"`
	Invalid map[string]string `json:"invalid"`
}

func (s *server) libraryAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		cookie, err := r.Cookie("tmux_agent")
		if err != nil || !s.ok(cookie.Value) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	lib, err := library.Load(s.dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.Method == http.MethodGet {
		out := libraryListing{Invalid: map[string]string{}}
		for _, v := range lib.Skills {
			out.Skills = append(out.Skills, v)
		}
		home, _ := os.UserHomeDir()
		for _, v := range library.ListNative(home, s.dir) {
			if _, shadowed := lib.Skills[v.Name]; !shadowed {
				out.Skills = append(out.Skills, v)
			}
		}
		state, _ := os.UserConfigDir()
		state = filepath.Join(state, "tagent")
		for _, v := range lib.Servers {
			v.Approved, _ = library.Approved(state, v)
			out.Servers = append(out.Servers, v)
		}
		for k, v := range lib.Invalid {
			out.Invalid[k] = v.Error()
		}
		json.NewEncoder(w).Encode(out)
		return
	}
	kind, name := r.PathValue("kind"), r.PathValue("name")
	if name != "" && !libraryName.MatchString(name) {
		http.Error(w, "invalid library name", http.StatusBadRequest)
		return
	}
	state, _ := os.UserConfigDir()
	state = filepath.Join(state, "tagent")
	if r.Method == http.MethodPost && kind == "approve" {
		v, ok := lib.Servers[name]
		if !ok {
			http.Error(w, "MCP server not found", 404)
			return
		}
		if err := library.Approve(state, v); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodPost {
		var body struct {
			Type   string         `json:"type"`
			Skill  string         `json:"skill"`
			Server library.Server `json:"server"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body) != nil {
			http.Error(w, "invalid JSON", 400)
			return
		}
		base := filepath.Join(s.dir, ".tasks", "library")
		switch body.Type {
		case "skill":
			if !libraryName.MatchString(body.Skill) {
				http.Error(w, "invalid skill", 400)
				return
			}
			p := filepath.Join(base, "skills", body.Skill)
			if err := os.MkdirAll(p, 0755); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte(body.Skill), 0644); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		case "mcp":
			v := body.Server
			if !libraryName.MatchString(v.Name) || v.Name == "" || v.Command == "" {
				http.Error(w, "invalid server", 400)
				return
			}
			for k, val := range v.Env {
				if !libraryEnvRef.MatchString(val) {
					http.Error(w, "env "+k+" must reference ${VAR}", 400)
					return
				}
			}
			lib.Servers[v.Name] = v
			// Marshal the complete validated server list as YAML using the library schema.
			var entries []library.Server
			for _, server := range lib.Servers {
				entries = append(entries, server)
			}
			data, err := yaml.Marshal(struct {
				Servers []library.Server `yaml:"servers"`
			}{entries})
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			if err = os.MkdirAll(base, 0755); err == nil {
				err = os.WriteFile(filepath.Join(base, "mcp.yml"), data, 0644)
			}
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		default:
			http.Error(w, "type must be skill or mcp", 400)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodDelete {
		base := filepath.Join(s.dir, ".tasks", "library")
		if kind == "skills" {
			if _, ok := lib.Skills[name]; !ok {
				http.Error(w, "skill not found", 404)
				return
			}
			err = os.RemoveAll(filepath.Join(base, "skills", name))
		} else if kind == "mcp" {
			if _, ok := lib.Servers[name]; !ok {
				http.Error(w, "server not found", 404)
				return
			}
			delete(lib.Servers, name)
			var entries []library.Server
			for _, v := range lib.Servers {
				entries = append(entries, v)
			}
			var data []byte
			data, err = yaml.Marshal(struct {
				Servers []library.Server `yaml:"servers"`
			}{entries})
			if err == nil {
				err = os.WriteFile(filepath.Join(base, "mcp.yml"), data, 0644)
			}
		} else {
			http.Error(w, "unknown library type", 400)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "method not allowed", 405)
}
