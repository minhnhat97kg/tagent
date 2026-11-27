package tasks

import (
	"fmt"
	"strings"
)

// RoleContext limits what a role's agent can load and use. An unset field keeps
// the harness default; an empty list means none. Args is the only mapping to
// harness flags, and it rejects what a harness cannot express.
type RoleContext struct {
	Skills *[]string `yaml:"skills,omitempty" json:"skills,omitempty"`
	MCP    *[]string `yaml:"mcp,omitempty" json:"mcp,omitempty"`
	Tools  []string  `yaml:"tools,omitempty" json:"tools,omitempty"`
	Allow  []string  `yaml:"allow,omitempty" json:"allow,omitempty"`
	Deny   []string  `yaml:"deny,omitempty" json:"deny,omitempty"`
}

// ScoutContext is the scout job's preset: read and search, the ticket tools
// (no task_add), and only the tagent MCP server.
func ScoutContext() RoleContext {
	none, mcp := []string{}, []string{"tagent"}
	ticket := []string{"mcp__tagent__task_show", "mcp__tagent__task_move", "mcp__tagent__task_comment"}
	// Checked live on claude 2.1.332: --tools must name the MCP tools too, and then only those reach the agent.
	return RoleContext{Skills: &none, MCP: &mcp, Tools: append([]string{"Read", "Grep", "Glob"}, ticket...), Allow: ticket, Deny: []string{"mcp__tagent__task_add"}}
}

// Args returns the harness flags for c. mcpFile is the generated MCP config
// that a non-empty MCP list points at.
func (c RoleContext) Args(harness, mcpFile string) ([]string, error) {
	var out []string
	unsupported := func(field string) error {
		return fmt.Errorf("%s is not supported for %s", field, harness)
	}
	switch harness {
	case "claude":
		if c.Skills != nil && len(*c.Skills) == 0 {
			out = append(out, "--disable-slash-commands")
		}
		if c.MCP != nil {
			out = append(out, "--strict-mcp-config")
			if len(*c.MCP) > 0 {
				out = append(out, "--mcp-config="+mcpFile)
			}
		}
		if len(c.Tools) > 0 {
			out = append(out, "--tools="+strings.Join(c.Tools, ","))
		}
		if len(c.Allow) > 0 {
			out = append(out, "--allowedTools="+strings.Join(c.Allow, ","))
		}
		if len(c.Deny) > 0 {
			out = append(out, "--disallowedTools="+strings.Join(c.Deny, ","))
		}
	case "pi":
		if c.Skills != nil && len(*c.Skills) == 0 {
			out = append(out, "--no-skills")
		}
		if c.MCP != nil {
			if len(*c.MCP) > 0 {
				return nil, unsupported("mcp server list (use none)")
			}
			out = append(out, "--no-mcp")
		}
		if len(c.Tools) > 0 {
			out = append(out, "--tools="+strings.Join(c.Tools, ","))
		}
		if len(c.Allow) > 0 {
			return nil, unsupported("allow")
		}
		if len(c.Deny) > 0 {
			out = append(out, "--exclude-tools="+strings.Join(c.Deny, ","))
		}
	default:
		if c.Skills != nil || c.MCP != nil || len(c.Tools) > 0 || len(c.Allow) > 0 || len(c.Deny) > 0 {
			return nil, fmt.Errorf("role context is not supported for %s", harness)
		}
	}
	return out, nil
}

// Set reports whether any field restricts the harness.
func (c RoleContext) Set() bool {
	return c.Skills != nil || c.MCP != nil || len(c.Tools) > 0 || len(c.Allow) > 0 || len(c.Deny) > 0
}
