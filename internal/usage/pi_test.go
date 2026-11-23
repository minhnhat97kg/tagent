package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPi(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PI_CODING_AGENT_DIR", root)
	dir := filepath.Join(root, "sessions", "project")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "timestamp_s1.jsonl")
	data := `{"type":"session","id":"s1"}
{"type":"message","message":{"role":"assistant","usage":{"input":10,"output":5,"cacheRead":20,"cacheWrite":3,"totalTokens":38}}}
not json
{"type":"message","message":{"role":"user","usage":{"totalTokens":999}}}
{"type":"message","message":{"role":"assistant"}}
{"type":"message","message":{"role":"assistant","usage":{"input":2,"output":1,"totalTokens":3}}}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	want := Count{Input: 12, Output: 6, CacheRead: 20, CacheWrite: 3, Total: 41}
	if got, ok := Read("pi", "s1"); !ok || got != want {
		t.Fatalf("got %+v, %v; want %+v", got, ok, want)
	}
	if _, ok := Read("pi", "missing"); ok {
		t.Fatal("missing session reported usage")
	}
	for _, tc := range []struct {
		data      string
		available bool
	}{
		{`{"type":"session","id":"wrong"}`, false},
		{`{"type":"session","id":"s1"}`, false},
		{`{"type":"session","id":"s1"}
{"type":"message","message":{"role":"assistant","usage":{"totalTokens":0}}}`, true},
	} {
		if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		_, ok := Read("pi", "s1")
		if ok != tc.available {
			t.Fatalf("availability %v for %s", ok, tc.data)
		}
	}
}
