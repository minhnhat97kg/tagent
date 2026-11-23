package usage

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLastAssistantText(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	fixtures := map[string]string{
		".claude/projects/project/s1.jsonl": `{"type":"user","uuid":"u","parentUuid":null,"sessionId":"s1","message":{"role":"user","content":"question"}}
{"type":"assistant","uuid":"a","parentUuid":"u","sessionId":"s1","message":{"role":"assistant","id":"m1","content":[{"type":"thinking","thinking":"secret"},{"type":"text","text":"claude reply"},{"type":"tool_use","name":"bash"}]}}
`,
		".codex/sessions/2026/rollout_s1.jsonl": `{"type":"response_item","payload":{"role":"assistant","content":[{"type":"output_text","text":"old"}]}}
{"type":"response_item","payload":{"role":"user","content":[{"type":"input_text","text":"not user"}]}}
{"type":"response_item","payload":{"role":"assistant","content":[{"type":"reasoning","text":"not reasoning"},{"type":"output_text","text":"codex reply"}]}}
{"type":"response_item","payload":{"type":"function_call","arguments":"not tool"}}
`,
		".pi/agent/sessions/project/2026-01-01_s1.jsonl": `{"type":"session","id":"s1"}
{"type":"message","id":"u","parentId":null,"message":{"role":"user","content":[{"type":"text","text":"question"}]}}
{"type":"message","id":"a","parentId":"u","message":{"role":"assistant","content":[{"type":"text","text":"pi reply"},{"type":"toolCall","name":"bash"}]}}
{"type":"message","id":"abandoned","parentId":"a","message":{"role":"assistant","content":[{"type":"text","text":"wrong branch"}]}}
{"type":"message","id":"leaf","parentId":"a","message":{"role":"toolResult","content":[{"type":"text","text":"not result"}]}}
`,
	}
	for name, text := range fixtures {
		path := filepath.Join(home, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, h := range []string{"claude", "codex", "pi"} {
		if got := LastAssistantText(h, "s1"); got != h+" reply" {
			t.Fatalf("%s: %q", h, got)
		}
		if got := LastAssistantText(h, "missing"); got != "" {
			t.Fatal(got)
		}
	}
	piPath := filepath.Join(home, ".pi/agent/sessions/project/2026-01-01_s1.jsonl")
	f, err := os.OpenFile(piPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{\"type\":\"message\",\"id\":\"new-user\",\"parentId\":\"u\",\"message\":{\"role\":\"user\",\"content\":\"new branch\"}}\n")
	f.Close()
	if got := ReadAssistant("pi", "s1"); !got.Available || got.Text != "" {
		t.Fatal("abandoned answer on unanswered branch", got)
	}
	os.WriteFile(piPath, []byte("{\"type\":\"session\",\"id\":\"wrong-id\"}\n"+fixtures[".pi/agent/sessions/project/2026-01-01_s1.jsonl"]), 0600)
	if got := ReadAssistant("pi", "s1"); got.Text != "" || !got.Available {
		t.Fatal("identity mismatch", got)
	}
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Log("sqlite3 unavailable; JSONL readers passed")
		return
	}
	db := openCodeDB()
	os.MkdirAll(filepath.Dir(db), 0700)
	sql := `create table message(id text,session_id text,time_created integer,data text);create table part(id text,message_id text,time_created integer,data text);
insert into message values('m1','s1',1,'{"role":"assistant"}'),('m2','s1',2,'{"role":"assistant"}'),('m3','s1',0,'{"role":"user"}');
insert into part values('p1','m1',1,'{"type":"text","text":"old"}'),('p2','m2',2,'{"type":"text","text":"opencode reply"}'),('p3','m2',3,'{"type":"tool","text":"not tool"}'),('p4','m3',4,'{"type":"text","text":"not user"}');`
	if out, err := exec.Command("sqlite3", db, sql).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got := LastAssistantText("opencode", "s1"); got != "opencode reply" {
		t.Fatal(got)
	}
	if got := LastAssistantText("opencode", "s1' OR 1=1 --"); got != "" {
		t.Fatal("session escaped incorrectly", got)
	}
}
