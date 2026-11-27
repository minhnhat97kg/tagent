// Package library loads the repository's shared skill and MCP library.
package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var skillName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var envRef = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)

// Skill is a skill directory and its declared name.
type Skill struct {
	Name   string `yaml:"name"`
	Path   string `json:"path"`
	Source string `json:"source"`
}

// Server is an MCP server definition. Env values are references, never secret values.
type Server struct {
	Name     string            `yaml:"name" json:"name"`
	Command  string            `yaml:"command" json:"command"`
	Args     []string          `yaml:"args" json:"args"`
	Env      map[string]string `yaml:"env" json:"env"`
	Approved bool              `yaml:"-" json:"approved,omitempty"`
}

// Library contains valid entries and per-entry errors so one malformed item does not hide others.
type Library struct {
	Skills  map[string]Skill
	Servers map[string]Server
	Invalid map[string]error
}

func Load(root string) (*Library, error) {
	l := &Library{Skills: map[string]Skill{}, Servers: map[string]Server{}, Invalid: map[string]error{}}
	base := filepath.Join(root, ".tasks", "library")
	entries, err := os.ReadDir(filepath.Join(base, "skills"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(base, "skills", e.Name())
			b, er := os.ReadFile(filepath.Join(p, "SKILL.md"))
			if er != nil {
				l.Invalid["skill:"+e.Name()] = er
				continue
			}
			name := e.Name()
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "name:") {
					name = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "name:")), "\"'")
					break
				}
			}
			if !skillName.MatchString(name) { // the name becomes a file name in generated homes
				l.Invalid["skill:"+e.Name()] = fmt.Errorf("skill name %q: use letters, digits, ., - and _", name)
				continue
			}
			l.Skills[name] = Skill{Name: name, Path: p, Source: "library"}
		}
	}
	b, er := os.ReadFile(filepath.Join(base, "mcp.yml"))
	if er == nil {
		var doc yaml.Node
		if er = yaml.Unmarshal(b, &doc); er != nil {
			l.Invalid["mcp"] = er
			return l, nil
		}
		var items *yaml.Node
		if len(doc.Content) > 0 {
			root := doc.Content[0]
			if root.Kind != yaml.MappingNode {
				l.Invalid["mcp"] = errors.New("mcp.yml must be a mapping with a servers list")
				return l, nil
			}
			for i := 0; i+1 < len(root.Content); i += 2 {
				if doc.Content[0].Content[i].Value == "servers" {
					items = doc.Content[0].Content[i+1]
					break
				}
			}
		}
		if items == nil || items.Kind != yaml.SequenceNode {
			l.Invalid["mcp"] = errors.New("mcp.yml servers must be a list")
			return l, nil
		}
		for i, item := range items.Content {
			var s Server
			if er := item.Decode(&s); er != nil {
				l.Invalid[fmt.Sprintf("mcp:entry-%d", i)] = er
				continue
			}
			if s.Name == "" || s.Command == "" {
				l.Invalid["mcp:"+s.Name] = errors.New("name and command are required")
				continue
			}
			invalid := ""
			for k, v := range s.Env {
				if !envRef.MatchString(v) {
					invalid = k
					break
				}
			}
			if invalid != "" {
				l.Invalid["mcp:"+s.Name] = fmt.Errorf("env %s must reference ${VAR}", invalid)
				continue
			}
			l.Servers[s.Name] = s
		}
	} else if !os.IsNotExist(er) {
		return l, er
	}
	return l, nil
}

// Fingerprint identifies the exact server configuration requiring approval.
func Fingerprint(s Server) string {
	b, _ := yaml.Marshal(s)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Approve records the current server fingerprint in the user state directory.
func Approve(stateDir string, s Server) error {
	return updateApproval(stateDir, s.Name, Fingerprint(s))
}

// Approved reports whether this exact server definition was approved.
func Approved(stateDir string, s Server) (bool, error) {
	b, err := os.ReadFile(filepath.Join(stateDir, "library-approvals.json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var approvals map[string]string
	if json.Unmarshal(b, &approvals) != nil {
		return false, nil
	}
	return approvals[s.Name] == Fingerprint(s), nil
}

func updateApproval(stateDir, name, fingerprint string) error {
	path := filepath.Join(stateDir, "library-approvals.json")
	approvals := map[string]string{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &approvals)
	}
	approvals[name] = fingerprint
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(approvals)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}
