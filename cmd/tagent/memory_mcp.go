package main

import (
	"bytes"
	"fmt"
	"os"

	"tagent/internal/tasks"
)

// memSpec maps one memory_* MCP tool onto a `tagent memory` subcommand.
// Required and positional args are passed positionally; the rest as --flag=value.
type memSpec struct {
	name, desc, sub string
	required        []string
	positional      []string
	props           map[string]any
}

// memoryMCPTools exposes the memory graph. There is deliberately no delete tool.
func memoryMCPTools() []mcpTool {
	scope := str("repo (default) or shared; memory_search defaults to both")
	specs := []memSpec{
		{"memory_search", "Search the memory graph (repo and shared) for nodes matching all words.", "search",
			[]string{"query"}, []string{"query"}, map[string]any{"query": str("Words to match"), "limit": str("Max results, 1-50"), "scope": scope}},
		{"memory_show", "Show one memory node in full, with its broken-ref flag.", "show",
			[]string{"id"}, []string{"id"}, map[string]any{"id": str("Node id"), "scope": scope}},
		{"memory_related", "List nodes linked to a node, breadth-first, up to depth 3.", "related",
			[]string{"id"}, []string{"id"}, map[string]any{"id": str("Node id"), "depth": str("1-3, default 1"), "limit": str("Max results, default 20"), "scope": scope}},
		{"memory_add_node", "Create a memory node (text, markdown, code, json, yaml, url, file or image) that you own.", "add-node",
			[]string{"label", "kind"}, nil, map[string]any{"label": str("Short label"), "kind": str("Node kind"), "content": str("Body for text kinds and url"), "ref": str("Repo file path for file kind, or source image path"), "lang": str("Language for code"), "tags": str("Comma-separated tags"), "scope": scope}},
		{"memory_add_link", "Link two existing nodes with a label.", "add-link",
			[]string{"from", "to", "label"}, []string{"from", "to", "label"}, map[string]any{"from": str("From node id"), "to": str("To node id"), "label": str("Link label"), "scope": scope}},
		{"memory_edit_node", "Edit a memory node you created (not one the user wrote).", "edit-node",
			[]string{"id"}, []string{"id"}, map[string]any{"id": str("Node id"), "label": str("New label"), "content": str("New content"), "ref": str("New file path"), "lang": str("Language"), "tags": str("Comma-separated tags"), "scope": scope}},
		{"memory_edit_link", "Relabel a link you created (not one the user wrote).", "edit-link",
			[]string{"id", "label"}, []string{"id"}, map[string]any{"id": str("Link id"), "label": str("New label"), "scope": scope}},
		{"memory_check", "Report corrupt records, broken file/image refs and dangling links. Changes nothing.", "check",
			nil, nil, map[string]any{"scope": scope}},
	}
	var out []mcpTool
	for _, sp := range specs {
		sp := sp
		out = append(out, mcpTool{
			Name:        sp.name,
			Description: sp.desc,
			InputSchema: schema(sp.required, sp.props),
			run: func(wd string, a map[string]any) (string, error) {
				for _, k := range sp.required {
					if argStr(a, k) == "" {
						return "", fmt.Errorf("missing argument %q", k)
					}
				}
				return runMemoryMCP(sp, a)
			},
		})
	}
	return out
}

func runMemoryMCP(sp memSpec, a map[string]any) (string, error) {
	args := []string{sp.sub}
	posSet := map[string]bool{}
	for _, k := range sp.positional {
		posSet[k] = true
		args = append(args, argStr(a, k))
	}
	for k, v := range a {
		if posSet[k] || v == nil {
			continue
		}
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		args = append(args, fmt.Sprintf("--%s=%v", k, v))
	}
	var out, errb bytes.Buffer
	code := runMemory(args, mcpActor(), &out, &errb)
	if code != exitOK {
		msg := errb.String()
		if msg == "" {
			msg = fmt.Sprintf("memory exited %d", code)
		}
		return "", fmt.Errorf("%s", msg)
	}
	return out.String(), nil
}

// mcpActor is the caller's role. An MCP call from inside a tmux pane with no
// resolvable role is treated as an agent, so it can never act as the user.
func mcpActor() string {
	actor := tasks.CallerActor()
	if actor == "user" && os.Getenv("TMUX_PANE") != "" {
		return "agent"
	}
	return actor
}
