package usage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReplyBudgets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".pi", "agent", "sessions", "project")
	os.MkdirAll(dir, 0700)
	path := filepath.Join(dir, "date_large.jsonl")
	header := `{"type":"session","id":"large"}` + "\n"
	old := `{"type":"message","id":"old","parentId":null,"message":{"role":"assistant","content":[{"type":"text","text":"abandoned"}]}}` + "\n"
	padding := `{"type":"custom","data":"` + strings.Repeat("x", 2048) + `"}` + "\n"
	// The only ancestor of the final tool result is outside the bounded tail.
	// An unrelated recent assistant must not be substituted for that missing ancestor.
	text := header + old + strings.Repeat(padding, replyBytes/len(padding)+100) + old +
		`{"type":"message","id":"leaf","parentId":"outside-budget","message":{"role":"toolResult","content":[{"type":"text","text":"tool output"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	got := ReadAssistant("pi", "large")
	if !got.Available || got.Text != "" || time.Since(started) > time.Second {
		t.Fatal("tail/branch budget failed", got)
	}
	// Even tiny records cannot create an unbounded entry map or scanning loop.
	os.WriteFile(path, []byte(header+strings.Repeat(old, replyRecords+1)), 0600)
	if got = ReadAssistant("pi", "large"); !got.Available || got.Text != "" {
		t.Fatal("record budget did not fail closed", got)
	}
	// Oversized individual records are bounded separately from total bytes.
	os.WriteFile(path, []byte(header+`{"type":"custom","data":"`+strings.Repeat("x", 300<<10)+`"}`+"\n"+old), 0600)
	if got = ReadAssistant("pi", "large"); !got.Available || got.Text != "" {
		t.Fatal("record byte budget", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got = readAssistant(ctx, "pi", "large"); !got.Available || got.Text != "" {
		t.Fatal("deadline must fail closed", got)
	}
}

func TestSessionDiscoveryBudget(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < discoveryEntries+1; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("%05d", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if path, exhausted := findSession(ctx, root, "missing.jsonl"); path != "" || !exhausted {
		t.Fatal("discovery entry budget", path, exhausted)
	}
	cancel()
	if _, exhausted := findSession(ctx, root, "also-missing.jsonl"); !exhausted {
		t.Fatal("discovery deadline")
	}
}
