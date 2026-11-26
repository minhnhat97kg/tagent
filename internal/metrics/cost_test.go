package metrics

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tagent/internal/tasks"
	"tagent/internal/usage"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestCostOfExactSplit(t *testing.T) {
	// 1M input, 100K output, 2M cache read, 500K cache write on Sonnet 5.5.
	c := usage.Count{Input: 1_000_000, Output: 100_000, CacheRead: 2_000_000, CacheWrite: 500_000, Total: 3_600_000}
	m := costOf(c, prices["claude-sonnet-5-5"])
	want := 1*2.0 + 0.1*10 + 2*0.10 + 0.5*2.50
	if m.Low != m.High || !near(m.Low, want) {
		t.Fatalf("cost = %+v, want exact %v", m, want)
	}
}

func TestCostOfRangeWhenSplitUnknown(t *testing.T) {
	// Total only, no input/output split: uncached rest is 1M tokens.
	c := usage.Count{CacheRead: 1_000_000, Total: 2_000_000}
	m := costOf(c, prices["claude-opus-5-5"])
	if !near(m.Low, 0.20+1*4) || !near(m.High, 0.20+1*20) {
		t.Fatalf("range = %+v, want [4.20, 20.20]", m)
	}
}

func TestPriceOfOverrideAndUnknown(t *testing.T) {
	cfg := tasks.BoardConfig{Pricing: map[string]tasks.ModelPrice{"new-model": {Input: 1, Output: 2}}}
	if _, ok := priceOf(cfg, "new-model"); !ok {
		t.Fatal("board.yaml pricing not used")
	}
	if _, ok := priceOf(cfg, "mystery-model"); ok {
		t.Fatal("unknown model must have no price")
	}
	if _, ok := priceOf(cfg, ""); ok {
		t.Fatal("empty model must have no price")
	}
	if p, ok := priceOf(cfg, "sonnet"); !ok || p.Input != 2 {
		t.Fatalf("alias sonnet not priced as current Sonnet: %+v %v", p, ok)
	}
}

func TestLinesShowCostAndUnknownModel(t *testing.T) {
	u := TicketUsage{
		Jobs:     []JobUsage{{Role: "dev", Model: "claude-haiku-5-5", Has: true, Count: usage.Count{Input: 1000, Output: 1000}, Loops: 1, Cost: &Money{Low: 0.0006, High: 0.0006}}},
		Total:    usage.Count{Input: 1000, Output: 1000, Total: 2000},
		Cost:     &Money{Low: 0.0006, High: 0.0006},
		Loops:    1,
		Unpriced: true,
	}
	got := strings.Join(u.Lines(func(n int64) string { return "N" }), "\n")
	if !strings.Contains(got, "API-equivalent") || !strings.Contains(got, "unknown model") {
		t.Fatalf("lines missing cost label or unknown-model note:\n%s", got)
	}
	if !strings.Contains(got, "≈<$0.01") {
		t.Fatalf("job cost missing:\n%s", got)
	}
}

func TestBoardYAMLPricingParses(t *testing.T) {
	cfg, err := tasks.ParseBoardConfig([]byte("pricing:\n  my-model: {input: 1, output: 4, cache_read: 0.1, cache_write: 1.25}\n"))
	if err != nil {
		t.Fatalf("pricing key rejected: %v", err)
	}
	if p, ok := priceOf(cfg, "my-model"); !ok || p.Output != 4 || p.CacheWrite != 1.25 {
		t.Fatalf("pricing override not loaded: %+v %v", p, ok)
	}
}

func TestCostOfHourCacheAndLongPrompts(t *testing.T) {
	// 1M cache write, 600K of it for an hour.
	m := costOf(usage.Count{CacheWrite: 1_000_000, CacheWrite1h: 600_000, Total: 1_000_000}, prices["claude-opus-5-5"])
	if want := 0.4*5 + 0.6*8; !near(m.Low, want) {
		t.Fatalf("1h cache = %+v, want %v", m, want)
	}
	// Haiku: 2M input of which 1M came in prompts over 100K; Sonnet has no such tier.
	c := usage.Count{Input: 2_000_000, Long: usage.Split{In: 1_000_000}, Total: 2_000_000}
	if m := costOf(c, prices["claude-haiku-5-5"]); !near(m.Low, 0.10+0.50) {
		t.Fatalf("long haiku = %+v, want 0.60", m)
	}
	if m := costOf(c, prices["claude-sonnet-5-5"]); !near(m.Low, 4) {
		t.Fatalf("sonnet = %+v, want 4", m)
	}
}

func TestWithRatesReadsOldRecordsFromTheTranscript(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "projects", "-p"), 0o755)
	os.WriteFile(filepath.Join(dir, "projects", "-p", "old.jsonl"), []byte(`{"type":"assistant","message":{"id":"a","usage":{"cache_creation_input_tokens":10,"cache_creation":{"ephemeral_1h_input_tokens":10}}}}`+"\n"), 0o644)
	old := Session{ID: "old", Harness: "claude", Usage: usage.Count{CacheWrite: 10, Total: 10}}
	if got := withRates(old); got.CacheWrite1h != 10 {
		t.Fatalf("old record not filled: %+v", got)
	}
	old.Usage.Total = 99 // the transcript no longer matches: keep what was recorded
	if got := withRates(old); got.CacheWrite1h != 0 {
		t.Fatalf("mismatch replaced the record: %+v", got)
	}
}
