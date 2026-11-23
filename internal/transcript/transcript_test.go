package transcript

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// home points HOME, CLAUDE_CONFIG_DIR and CODEX_HOME at a temp tree.
func home(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(h, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(h, ".codex"))
	return h
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func claudePath(h string) string { return filepath.Join(h, ".claude", "projects", "p", "s1.jsonl") }

func TestParseClaude(t *testing.T) {
	cases := map[string]struct {
		line string
		want []string // role|text or role|tool name|label
	}{
		"text and string content":           {`{"type":"user","uuid":"u","sessionId":"s1","message":{"role":"user","content":"hi"}}`, []string{"user|hi"}},
		"thinking kept, empty skipped":      {`{"type":"assistant","uuid":"a","message":{"role":"assistant","content":[{"type":"thinking","thinking":""},{"type":"thinking","thinking":"plan"},{"type":"text","text":"done"}]}}`, []string{"thinking|plan", "assistant|done"}},
		"tool call label":                   {`{"type":"assistant","uuid":"a","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/x/web/Board.tsx"}},{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"go test ./...\nmore"}}]}}`, []string{"tool|Edit Board.tsx", "tool|Bash go test ./..."}},
		"tool result":                       {`{"type":"user","uuid":"r","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"ok"}]}]}}`, []string{"tool|t1=ok"}},
		"meta and sidechain carry no items": {`{"type":"user","uuid":"m","isMeta":true,"message":{"role":"user","content":"caveat"}}`, nil},
		"attachments keep links only":       {`{"type":"attachment","uuid":"x","parentUuid":"u"}`, nil},
	}
	for name, c := range cases {
		rec, ok := parseClaude([]byte(c.line), "s1")
		if !ok {
			t.Fatalf("%s: no record", name)
		}
		var got []string
		for _, it := range rec.Items {
			switch {
			case it.Tool != nil && it.Tool.CallID != "" && it.Tool.Name == "":
				got = append(got, "tool|"+it.Tool.CallID+"="+it.Tool.Output)
			case it.Tool != nil:
				got = append(got, "tool|"+it.Tool.Label)
			default:
				got = append(got, it.Role+"|"+it.Text)
			}
		}
		if strings.Join(got, ";") != strings.Join(c.want, ";") {
			t.Fatalf("%s: got %q want %q", name, got, c.want)
		}
	}
}

func TestParsePi(t *testing.T) {
	rec, ok := parsePi([]byte(`{"type":"message","id":"a","parentId":"u","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hm"},{"type":"text","text":"reply"},{"type":"toolCall","id":"c1|x","name":"bash","arguments":{"command":"go test ./..."}}]}}`), "")
	if !ok || rec.Parent != "u" || len(rec.Items) != 3 {
		t.Fatalf("%+v", rec)
	}
	if rec.Items[2].Tool.Label != "bash go test ./..." || rec.Items[2].Tool.CallID != "c1|x" {
		t.Fatalf("%+v", rec.Items[2].Tool)
	}
	res, _ := parsePi([]byte(`{"type":"message","id":"r","parentId":"a","message":{"role":"toolResult","toolCallId":"c1|x","toolName":"bash","content":[{"type":"text","text":"PASS"}]}}`), "")
	if res.Items[0].Role != "tool" || res.Items[0].Tool.Output != "PASS" || res.Items[0].Tool.Name != "bash" {
		t.Fatalf("%+v", res.Items)
	}
	if _, ok := parsePi([]byte(`{"type":"session","id":"s1"}`), ""); ok {
		t.Fatal("header is not a record")
	}
}

func TestReadBranchCursorAndPartialLine(t *testing.T) {
	h := home(t)
	path := claudePath(h)
	line := func(uuid, parent, typ, text string) string {
		b, _ := json.Marshal(map[string]any{"type": typ, "uuid": uuid, "parentUuid": parent, "sessionId": "s1",
			"message": map[string]any{"role": typ, "content": text}})
		return string(b) + "\n"
	}
	// a2 is an abandoned answer to u1; the active branch is u1 a1 u2 a3.
	write(t, path, line("u1", "", "user", "q1")+line("a1", "u1", "assistant", "one")+line("a2", "u1", "assistant", "abandoned")+line("u2", "a1", "user", "q2")+line("a3", "u2", "assistant", "three"))
	ctx := context.Background()
	page, err := Read(ctx, "claude", "s1", "")
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, it := range page.Items {
		texts = append(texts, it.Text)
	}
	if strings.Join(texts, "|") != "q1|one|q2|three" {
		t.Fatalf("branch: %q", texts)
	}
	next, _ := Read(ctx, "claude", "s1", page.Items[1].ID)
	if next.Reset || len(next.Items) != 2 || next.Items[0].Text != "q2" || next.Cursor != page.Items[3].ID {
		t.Fatalf("after cursor: %+v", next)
	}
	if none, _ := Read(ctx, "claude", "s1", page.Cursor); len(none.Items) != 0 || none.Cursor != page.Cursor {
		t.Fatalf("caught up: %+v", none)
	}
	if gone, _ := Read(ctx, "claude", "s1", "unknown.0"); !gone.Reset || len(gone.Items) != 4 {
		t.Fatalf("unknown cursor must reset: %+v", gone)
	}
	// An unterminated last line is still being written: not shown, cursor not moved.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString(`{"type":"assistant","uuid":"a4","parentUuid":"a3","message":{"role":"assistant","content":"half`)
	f.Close()
	if partial, _ := Read(ctx, "claude", "s1", ""); partial.Items[len(partial.Items)-1].Text != "three" {
		t.Fatalf("partial line emitted: %+v", partial.Items)
	}
}

func TestReadPagesAndCaps(t *testing.T) {
	h := home(t)
	var b strings.Builder
	parent := ""
	for i := 0; i < 250; i++ {
		id := fmt.Sprintf("m%03d", i)
		p, _ := json.Marshal(parent)
		fmt.Fprintf(&b, `{"type":"user","uuid":%q,"parentUuid":%s,"message":{"role":"user","content":"msg %d"}}`+"\n", id, p, i)
		parent = id
	}
	write(t, claudePath(h), b.String())
	ctx := context.Background()
	first, _ := Read(ctx, "claude", "s1", "m000.0")
	if len(first.Items) != pageItems || !first.More || first.Items[0].Text != "msg 1" {
		t.Fatalf("first page: %d more=%v", len(first.Items), first.More)
	}
	rest, _ := Read(ctx, "claude", "s1", first.Cursor)
	if len(rest.Items) != 49 || rest.More {
		t.Fatalf("rest: %d", len(rest.Items))
	}
}

func TestReadUnsupportedAndMissing(t *testing.T) {
	home(t)
	ctx := context.Background()
	if _, err := Read(ctx, "codex", "s1", ""); err != ErrUnsupported {
		t.Fatal(err)
	}
	if _, err := Read(ctx, "opencode", "s1", ""); err != ErrUnsupported {
		t.Fatal(err)
	}
	if _, err := Read(ctx, "claude", "missing", ""); err != ErrUnreadable {
		t.Fatal(err)
	}
}

func TestPiSessionIdentity(t *testing.T) {
	h := home(t)
	path := filepath.Join(h, ".pi", "agent", "sessions", "p", "2026_s1.jsonl")
	write(t, path, `{"type":"session","id":"other"}`+"\n"+`{"type":"message","id":"u","parentId":null,"message":{"role":"user","content":"hi"}}`+"\n")
	if _, err := Read(context.Background(), "pi", "s1", ""); err != ErrUnreadable {
		t.Fatalf("header mismatch: %v", err)
	}
}
