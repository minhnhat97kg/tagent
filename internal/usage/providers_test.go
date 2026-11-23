package usage

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReadCodexLatestCumulativeUsage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	path := filepath.Join(home, "sessions", "2026", "11", "21", "rollout-session-a.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data := "" +
		`{"type":"token_usage_record","payload":{"thread_token_usage":{"input_tokens":10,"output_tokens":3,"total_tokens":13}}}` + "\n" +
		`{"type":"token_usage_record","payload":{"thread_token_usage":{"input_tokens":20,"output_tokens":5,"cached_input_tokens":4,"reasoning_output_tokens":2,"total_tokens":25}}}` + "\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, ok := Read("codex", "session-a")
	if !ok || c.Total != 25 || c.CacheRead != 4 || c.Reasoning != 2 {
		t.Fatalf("count=%+v ok=%v", c, ok)
	}
}

func TestReadOpenCodeSession(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 unavailable")
	}
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)
	db := filepath.Join(home, "opencode", "opencode.db")
	if err := os.MkdirAll(filepath.Dir(db), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sqlite3", db, "create table session(id text, tokens_input integer, tokens_output integer, tokens_reasoning integer, tokens_cache_read integer, tokens_cache_write integer); insert into session values('s1',10,3,2,4,1);")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %s: %v", out, err)
	}
	c, ok := Read("opencode", "s1")
	if !ok || c.Total != 13 || c.CacheRead != 4 || c.Reasoning != 2 {
		t.Fatalf("count=%+v ok=%v", c, ok)
	}
	if _, ok := Read("opencode", "missing"); ok {
		t.Fatal("missing session reported as zero")
	}
}
