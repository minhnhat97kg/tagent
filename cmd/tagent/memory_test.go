package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func memoryTestRepo(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	t.Chdir(dir)
}

func TestMemoryUsage(t *testing.T) {
	memoryTestRepo(t)
	var out, errb bytes.Buffer
	if code := cmdMemory(nil, &out, &errb); code != exitUsage {
		t.Fatalf("no subcommand: exit %d", code)
	}
	if code := cmdMemory([]string{"bogus"}, &out, &errb); code != exitUsage {
		t.Fatalf("unknown subcommand: exit %d", code)
	}
}

func TestMemoryBadScope(t *testing.T) {
	memoryTestRepo(t)
	var out, errb bytes.Buffer
	if code := cmdMemory([]string{"check", "--scope=both"}, &out, &errb); code != exitUsage {
		t.Fatalf("bad scope: exit %d", code)
	}
}

func TestMemoryAgentCannotDeleteViaCLI(t *testing.T) {
	memoryTestRepo(t)
	var out, errb bytes.Buffer
	if code := runMemory([]string{"add-node", "--label=a", "--kind=text", "--content=b"}, "pm", &out, &errb); code != exitOK {
		t.Fatalf("add: %d %s", code, errb.String())
	}
	var n struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out.Bytes(), &n); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := runMemory([]string{"rm-node", n.ID}, "pm", &out, &errb); code == exitOK {
		t.Fatal("agent rm-node succeeded")
	}
	if code := runMemory([]string{"rm-node", n.ID}, "user", &out, &errb); code != exitOK {
		t.Fatalf("user rm-node failed: %s", errb.String())
	}
}

func TestMemorySearchNoGraphIsEmpty(t *testing.T) {
	memoryTestRepo(t)
	var out, errb bytes.Buffer
	if code := runMemory([]string{"search", "anything"}, "user", &out, &errb); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if strings.TrimSpace(out.String()) != "null" {
		t.Fatalf("search output = %q", out.String())
	}
}

func TestMCPMemoryBadArgs(t *testing.T) {
	memoryTestRepo(t)
	resp := handleMCP(".", mcpRequest{
		ID:     json.RawMessage("1"),
		Method: "tools/call",
		Params: json.RawMessage(`{"name":"memory_show","arguments":{"id":5}}`),
	})
	res := resp.Result.(map[string]any)
	if res["isError"] != true {
		t.Fatalf("want isError for non-string id, got %v", res)
	}
}

func TestMCPMemoryAddAndSearch(t *testing.T) {
	memoryTestRepo(t)
	call := func(args string) map[string]any {
		resp := handleMCP(".", mcpRequest{
			ID:     json.RawMessage("1"),
			Method: "tools/call",
			Params: json.RawMessage(`{"name":"memory_add_node","arguments":` + args + `}`),
		})
		return resp.Result.(map[string]any)
	}
	if res := call(`{"label":"Deploy note","kind":"text","content":"ship on fridays"}`); res["isError"] != false {
		t.Fatalf("add failed: %v", res)
	}
	resp := handleMCP(".", mcpRequest{
		ID:     json.RawMessage("2"),
		Method: "tools/call",
		Params: json.RawMessage(`{"name":"memory_search","arguments":{"query":"fridays","scope":"repo"}}`),
	})
	text := resp.Result.(map[string]any)["content"].([]map[string]any)[0]["text"].(string)
	if !strings.Contains(text, "Deploy note") {
		t.Fatalf("search did not find node: %s", text)
	}
}
