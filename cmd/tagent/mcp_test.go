package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"tagent/internal/tasks"
)

func TestMCPToolsRoundTrip(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	t.Chdir(dir)
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}

	reqs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"task_add","arguments":{"title":"Fix login","description":"Details","priority":"high"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"task_show","arguments":{"id":"1"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"task_move","arguments":{"id":"1","to":"blocked","comment":"Which path?","outputs":{"count":2}}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"task_move","arguments":{"id":"1","to":"blocked","comment":"Which path?"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"task_comment","arguments":{"id":"1","text":"note"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"task_move","arguments":{"id":"999","to":"done"}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"nope"}`,
		`not json`,
	}
	var out bytes.Buffer
	if err := serveMCP(strings.NewReader(strings.Join(reqs, "\n")+"\n"), &out, dir); err != nil {
		t.Fatal(err)
	}
	var resp []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err, line)
		}
		resp = append(resp, m)
	}
	// 11 lines in, one notification (no reply): 10 responses.
	if len(resp) != 10 {
		t.Fatalf("got %d responses:\n%s", len(resp), out.String())
	}
	text := func(i int) string {
		res, _ := resp[i]["result"].(map[string]any)
		content, _ := res["content"].([]any)
		if len(content) == 0 {
			return ""
		}
		return content[0].(map[string]any)["text"].(string)
	}

	tools := resp[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) > 6 {
		t.Fatalf("%d tools, want <= 6", len(tools))
	}
	for _, tl := range tools {
		s := tl.(map[string]any)
		if s["description"] == "" || s["inputSchema"].(map[string]any)["required"] == nil {
			t.Fatalf("tool lacks description or required: %v", s)
		}
	}

	if got := text(2); got != "1" {
		t.Fatalf("task_add returned %q", got)
	}
	if got := text(3); !strings.Contains(got, "Fix login") || !strings.Contains(got, "Status: todo") {
		t.Fatalf("task_show: %q", got)
	}
	// Outputs need a workflow job that declares them; a plain ticket is a tool error.
	if res := resp[4]["result"].(map[string]any); res["isError"] != true || !strings.Contains(text(4), "outputs") {
		t.Fatalf("outputs on a plain ticket should be a tool error: %v", res)
	}
	if res := resp[5]["result"].(map[string]any); res["isError"] != false {
		t.Fatalf("task_move failed: %v", res)
	}
	task, err := tasks.Get(tasks.Scope(dir), "1")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "blocked" {
		t.Fatalf("move not applied: status=%q", task.Status)
	}
	if !strings.Contains(text(6), "Comment added") {
		t.Fatalf("task_comment: %q", text(6))
	}
	if res := resp[7]["result"].(map[string]any); res["isError"] != true {
		t.Fatalf("move of unknown ticket should be a tool error: %v", res)
	}
	if resp[8]["error"] == nil || resp[9]["error"] == nil {
		t.Fatalf("expected JSON-RPC errors for unknown method and bad JSON: %v %v", resp[8], resp[9])
	}
}
