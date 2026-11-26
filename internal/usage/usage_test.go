package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForSessionDedupesByMessageID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "projects", "-p"), 0o755)
	transcript := `{"type":"user","message":{"content":"hi"}}
{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":10,"output_tokens":1,"cache_read_input_tokens":100}}}
{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100}}}
{"type":"assistant","message":{"id":"m2","usage":{"input_tokens":20,"output_tokens":7,"cache_creation_input_tokens":3}}}
not json
`
	os.WriteFile(filepath.Join(dir, "projects", "-p", "s1.jsonl"), []byte(transcript), 0o644)
	got, ok := ForSession("s1")
	want := Tokens{In: 30, Out: 12, CacheRead: 100, CacheWrite: 3}
	if !ok || got != want {
		t.Fatalf("ForSession = %+v %v, want %+v", got, ok, want)
	}
	if got.Short() != "33 in · 12 out · 100 cached" {
		t.Errorf("Short = %q", got.Short())
	}
	if _, ok := ForSession("nope"); ok {
		t.Error("unknown session")
	}
}

func TestForSessionSplitsRates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "projects", "-p"), 0o755)
	// A short request, then one whose prompt (input + cache) is over 100K.
	transcript := `{"type":"assistant","message":{"id":"a","usage":{"input_tokens":5,"output_tokens":1,"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_1h_input_tokens":8}}}}
{"type":"assistant","message":{"id":"b","usage":{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":100000,"cache_creation_input_tokens":4,"cache_creation":{"ephemeral_1h_input_tokens":4}}}}
`
	os.WriteFile(filepath.Join(dir, "projects", "-p", "split.jsonl"), []byte(transcript), 0o644)
	got, ok := Read("claude", "split")
	if !ok || got.CacheWrite != 14 || got.CacheWrite1h != 12 || got.Long != (Split{In: 1, Out: 2, CacheRead: 100000, CacheWrite: 4, CacheWrite1h: 4}) {
		t.Fatalf("split = %+v %v", got, ok)
	}
}
