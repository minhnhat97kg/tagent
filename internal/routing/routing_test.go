package routing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tagent/internal/quota"
)

func pct(v float64) *float64 { return &v }

// window resets an hour from now unless reset is set.
func window(used float64) quota.Window {
	return quota.Window{Used: pct(used), Reset: []byte(`"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"`)}
}

func snap(claude, codex float64) quota.Snapshot {
	return quota.Snapshot{Claude: quota.Provider{Five: window(claude)}, Codex: quota.Provider{Five: window(codex)}}
}

var fast = Config{Tiers: map[string][]string{"fast": {"claude:haiku", "pi:openai-codex/gpt-6-luna"}}}

func TestSelectByQuota(t *testing.T) {
	tests := []struct {
		name         string
		snap         quota.Snapshot
		wantHarness  string
		wantModel    string
		wantReasonIn string
	}{
		{"under limit takes first", snap(10, 0), "claude", "haiku", ""},
		{"claude over limit falls to codex", snap(93, 10), "pi", "openai-codex/gpt-6-luna", "Claude 5h 93%"},
		{"unknown counts as available", quota.Snapshot{}, "claude", "haiku", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := fast.Select("fast", tt.snap)
			if err != nil {
				t.Fatal(err)
			}
			if c.Harness != tt.wantHarness || c.Model != tt.wantModel {
				t.Fatalf("got %s:%s", c.Harness, c.Model)
			}
			if !strings.Contains(c.Reason, tt.wantReasonIn) {
				t.Fatalf("reason %q", c.Reason)
			}
		})
	}
}

func TestExpiredWindowIgnored(t *testing.T) {
	s := quota.Snapshot{Claude: quota.Provider{Five: quota.Window{Used: pct(99), Reset: []byte(`"2000-01-01T00:00:00Z"`)}}}
	c, err := fast.Select("fast", s)
	if err != nil || c.Harness != "claude" {
		t.Fatalf("got %+v %v", c, err)
	}
}

func TestCodexMappingFromPi(t *testing.T) {
	cfg := Config{Tiers: map[string][]string{"x": {"pi:openai-codex/gpt-6.1-sol", "pi:anthropic/other"}}}
	cands, err := cfg.Candidates("x", snap(0, 95))
	if err != nil {
		t.Fatal(err)
	}
	if cands[0].State != "exhausted" || cands[1].State != "unknown" {
		t.Fatalf("states %+v", cands)
	}
}

func TestBlockWhenAllExhausted(t *testing.T) {
	_, err := fast.Select("fast", snap(95, 95))
	if err == nil || !strings.Contains(err.Error(), "Claude 5h 95%") || !strings.Contains(err.Error(), "earliest reset") {
		t.Fatalf("err %v", err)
	}
}

func TestFirstWhenAllExhausted(t *testing.T) {
	cfg := fast
	cfg.WhenExhausted = "first"
	c, err := cfg.Select("fast", snap(95, 95))
	if err != nil || c.Harness != "claude" || c.Model != "haiku" {
		t.Fatalf("got %+v %v", c, err)
	}
}

func TestUnknownTier(t *testing.T) {
	if _, err := fast.Select("smart", snap(0, 0)); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadRejectsBadConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "tagent", "routing.yaml")
	os.MkdirAll(filepath.Dir(path), 0700)
	for _, body := range []string{
		"limits: {gemini: 50}\n",
		"when_exhausted: wait\n",
		"tiers: {fast: [haiku]}\n",
		"tiers: {fast: [claude:]}\n",
		"tiers: {fast: []}\n",
		"unknown: 1\n",
	} {
		os.WriteFile(path, []byte(body), 0600)
		if _, err := Load(); err == nil {
			t.Fatalf("want error for %q", body)
		}
	}
	os.WriteFile(path, []byte("limits: {claude: 80}\ntiers: {fast: [claude:haiku]}\n"), 0600)
	c, err := Load()
	if err != nil || c.Limits["claude"] != 80 {
		t.Fatalf("got %+v %v", c, err)
	}
}
