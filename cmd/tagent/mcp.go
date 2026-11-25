package main

// `tagent mcp`: a stdio MCP server with four typed ticket tools for harnesses
// and small models. Each tool calls the same internal/tasks code as the CLI;
// nothing shells out. Newline-delimited JSON-RPC 2.0, stdlib only.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"tagent/internal/tasks"
	"tagent/internal/version"
)

type mcpRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type mcpResponse struct {
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
	JSONRPC string          `json:"jsonrpc"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	run         func(wd string, a map[string]any) (string, error)
}

func cmdMCP(stdout, stderr io.Writer) int {
	wd, _ := os.Getwd()
	if err := serveMCP(os.Stdin, stdout, wd); err != nil {
		fmt.Fprintln(stderr, "mcp:", err)
		return exitFailure
	}
	return exitOK
}

func serveMCP(in io.Reader, out io.Writer, wd string) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var req mcpRequest
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			if err := enc.Encode(mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &mcpError{-32700, "parse error"}}); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 { // notification: no response
			continue
		}
		if err := enc.Encode(handleMCP(wd, req)); err != nil {
			return err
		}
	}
	return sc.Err()
}

func handleMCP(wd string, req mcpRequest) mcpResponse {
	resp := mcpResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		pv := "2025-06-18"
		if p.ProtocolVersion == "2024-11-05" || p.ProtocolVersion == "2025-03-26" || p.ProtocolVersion == "2025-06-18" {
			pv = p.ProtocolVersion
		}
		resp.Result = map[string]any{
			"protocolVersion": pv,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "tagent", "version": version.Version},
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = &mcpError{-32602, "invalid params"}
			return resp
		}
		tool, ok := findTool(p.Name)
		if !ok {
			resp.Error = &mcpError{-32602, "unknown tool: " + p.Name}
			return resp
		}
		text, err := tool.run(wd, p.Arguments)
		if err != nil {
			text = err.Error()
		}
		resp.Result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": err != nil,
		}
	default:
		resp.Error = &mcpError{-32601, "method not found: " + req.Method}
	}
	return resp
}

func findTool(name string) (mcpTool, bool) {
	for _, t := range mcpTools() {
		if t.Name == name {
			return t, true
		}
	}
	return mcpTool{}, false
}

func schema(required []string, props map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

func mcpTools() []mcpTool {
	return []mcpTool{
		{
			Name:        "task_show",
			Description: "Show a ticket: its status, description, comments and handoffs.",
			InputSchema: schema([]string{"id"}, map[string]any{"id": str("Ticket id, e.g. 12")}),
			run: func(wd string, a map[string]any) (string, error) {
				scope := tasks.Scope(wd)
				t, err := tasks.Get(scope, argStr(a, "id"))
				if err != nil {
					return "", err
				}
				return briefView(scope, t)
			},
		},
		{
			Name:        "task_move",
			Description: "Finish your step: move the ticket to a next status with a short handoff and optional outputs.",
			InputSchema: schema([]string{"id", "to"}, map[string]any{
				"id":      str("Ticket id"),
				"to":      str("Next status, one of the Allowed next statuses in your prompt (or 'blocked')"),
				"comment": str("Short handoff: checks run, changed paths, next step"),
				"outputs": map[string]any{"type": "object", "description": "Requested outputs as key:value strings", "additionalProperties": map[string]any{"type": "string"}},
			}),
			run: func(wd string, a map[string]any) (string, error) {
				scope := tasks.Scope(wd)
				id, to := argStr(a, "id"), argStr(a, "to")
				outputs, err := argOutputs(a)
				if err != nil {
					return "", err
				}
				if err := tasks.Move(scope, id, to, argStr(a, "comment"), tasks.MutationOptions{Outputs: outputs}); err != nil {
					return "", err
				}
				if err := dispatchEntry(wd, scope, id, to); err != nil {
					return "", err
				}
				return fmt.Sprintf("Ticket %s moved to %s.", id, to), nil
			},
		},
		{
			Name:        "task_comment",
			Description: "Add a comment to a ticket. Use @role to notify another role.",
			InputSchema: schema([]string{"id", "text"}, map[string]any{
				"id":   str("Ticket id"),
				"text": str("Comment text"),
			}),
			run: func(wd string, a map[string]any) (string, error) {
				err := tasks.Comment(tasks.Scope(wd), argStr(a, "id"), tasks.CallerActor(), argStr(a, "text"), nil)
				var notice *tasks.CommentNotice
				if errors.As(err, &notice) {
					return "Comment saved. " + notice.Error(), nil
				}
				if err != nil {
					return "", err
				}
				return "Comment added.", nil
			},
		},
		{
			Name:        "task_add",
			Description: "Create a new ticket in todo. Returns its id.",
			InputSchema: schema([]string{"title"}, map[string]any{
				"title":       str("Short summary, under 80 characters"),
				"description": str("Details: context, steps, acceptance"),
				"to":          str("Comma-separated role names to assign"),
				"priority":    map[string]any{"type": "string", "enum": []string{"high", "normal", "low"}, "description": "Default normal"},
				"workflow":    str("Workflow name to attach; omit for default_workflow"),
			}),
			run: func(wd string, a map[string]any) (string, error) {
				var names []string
				if to := argStr(a, "to"); to != "" {
					names = strings.Split(to, ",")
				}
				opt := tasks.MutationOptions{Actor: tasks.CallerActor(), Workflow: argStr(a, "workflow"), Priority: argStr(a, "priority")}
				text := strings.TrimSpace(argStr(a, "title"))
				if d := strings.TrimSpace(argStr(a, "description")); d != "" {
					text += "\n\n" + d
				}
				t, err := tasks.Add(tasks.Scope(wd), text, names, opt)
				if err != nil {
					return "", err
				}
				return t.ID, nil
			},
		},
	}
}

func argStr(a map[string]any, k string) string {
	s, _ := a[k].(string)
	return s
}

// argOutputs accepts string values; numbers and booleans are stringified so a
// model that sends `"n": 3` is not rejected.
func argOutputs(a map[string]any) (map[string]string, error) {
	raw, ok := a["outputs"]
	if !ok || raw == nil {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("outputs must be an object of key: value")
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if k == "" {
			return nil, errors.New("output key must not be empty")
		}
		out[k] = fmt.Sprint(v)
	}
	return out, nil
}
