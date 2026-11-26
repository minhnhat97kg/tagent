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
		{"claude over limit falls to codex", snap(93, 10), "pi", "openai-codex/gpt-6-luna", "claude:haiku 5h 93% >= 90"},
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
	if err == nil || !strings.Contains(err.Error(), "claude:haiku 5h 95% >= 90") || !strings.Contains(err.Error(), "earliest reset") {
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

func TestSaveRoundTripAndRejects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	good := Config{Limits: map[string]float64{"claude": 80}, WhenExhausted: "first",
		Tiers: map[string][]string{"fast": {"claude:haiku", "pi:openai-codex/gpt-6-luna"}}}
	if err := Save(good); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil || c.Limits["claude"] != 80 || c.WhenExhausted != "first" || c.Tiers["fast"][1] != "pi:openai-codex/gpt-6-luna" {
		t.Fatalf("got %+v %v", c, err)
	}
	before, _ := os.ReadFile(Path())
	for _, bad := range []Config{
		{Tiers: map[string][]string{"my.tier": {"claude:haiku"}}},
		{Tiers: map[string][]string{"-x": {"claude:haiku"}}},
		{Tiers: map[string][]string{"fast": {}}},
		{Tiers: map[string][]string{"fast": {"haiku"}}},
	} {
		if err := Save(bad); err == nil {
			t.Fatalf("want error for %+v", bad)
		}
	}
	if after, _ := os.ReadFile(Path()); string(after) != string(before) {
		t.Fatal("rejected save changed the file")
	}
}

func TestParseThreshold(t *testing.T) {
	for _, s := range []string{"claude:opus", "claude:opus@70", "claude:opus@1", "claude:opus@100", "pi:vertex/x@20250514", "pi:vertex/x@20250514@70"} {
		if _, _, _, err := parse(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"claude:opus@0", "claude:opus@0.5", "claude:opus@101", "claude:opus@-1", "claude:opus@NaN", "claude:@70", "claude:"} {
		if _, _, _, err := parse(s); err == nil {
			t.Errorf("%s: want error", s)
		}
	}
	if h, m, u, _ := parse("pi:vertex/x@20250514@70"); h != "pi" || m != "vertex/x@20250514" || u != 70 {
		t.Fatalf("got %s %s %v", h, m, u)
	}
}

func TestThresholdSpill(t *testing.T) {
	smart := Config{Limits: map[string]float64{"claude": 80}, Tiers: map[string][]string{"smart": {"claude:opus@70", "pi:openai-codex/gpt-6.1-sol"}}}
	weekOnly := quota.Snapshot{Claude: quota.Provider{Five: window(10), Week: window(75)}}
	for _, tt := range []struct {
		name string
		snap quota.Snapshot
		want string
	}{
		{"under threshold stays", snap(69, 0), "claude:opus@70"},
		{"at threshold spills", snap(70, 0), "pi:openai-codex/gpt-6.1-sol"},
		{"7d alone spills", weekOnly, "pi:openai-codex/gpt-6.1-sol"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := smart.Select("smart", tt.snap)
			if err != nil || c.Ref != tt.want {
				t.Fatalf("got %+v %v", c, err)
			}
		})
	}
	if c, _ := smart.Select("smart", snap(70, 0)); c.Reason != "claude:opus 5h 70% >= 70" {
		t.Fatalf("got %+v", c)
	}
	cands, _ := smart.Candidates("smart", snap(85, 0))
	if cands[0].Threshold != 70 || cands[0].Used == nil || *cands[0].Used != 85 || cands[1].Threshold != 90 {
		t.Fatalf("candidates %+v", cands)
	}
	// own threshold beats the provider limit: 93 stays on @95 although limits say 80
	high := Config{Limits: map[string]float64{"claude": 80}, Tiers: map[string][]string{"x": {"claude:haiku@95"}}}
	if c, err := high.Select("x", snap(93, 0)); err != nil || c.Ref != "claude:haiku@95" || c.Model != "haiku" {
		t.Fatalf("got %+v %v", c, err)
	}
	// no threshold: 85 is past limits 80, so it spills
	plain := Config{Limits: map[string]float64{"claude": 80}, Tiers: map[string][]string{"x": {"claude:opus", "pi:openai-codex/gpt-6.1-sol"}}}
	if c, _ := plain.Select("x", snap(85, 0)); c.Ref != "pi:openai-codex/gpt-6.1-sol" {
		t.Fatalf("limit fallback: got %+v", c)
	}
}

func TestFirstFallbackReason(t *testing.T) {
	cfg := Config{WhenExhausted: "first", Tiers: map[string][]string{"fast": {"claude:haiku", "pi:openai-codex/gpt-6-luna"}}}
	c, err := cfg.Select("fast", snap(95, 95))
	if err != nil || c.Ref != "claude:haiku" || c.Reason != "claude:haiku 5h 95% >= 90; pi:openai-codex/gpt-6-luna 5h 95% >= 90; all exhausted, running first" {
		t.Fatalf("got %+v %v", c, err)
	}
}

func TestThresholdRoundTripKeepsOldFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "tagent", "routing.yaml")
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("tiers:\n  fast:\n    - claude:haiku\n    - pi:openai-codex/gpt-6-luna\n"), 0600)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	c.Tiers["fast"][0] = "claude:haiku@70"
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	c, err = Load()
	if err != nil || c.Tiers["fast"][0] != "claude:haiku@70" || c.Tiers["fast"][1] != "pi:openai-codex/gpt-6-luna" {
		t.Fatalf("got %+v %v", c, err)
	}
	if err := Save(Config{Tiers: map[string][]string{"fast": {"claude:haiku@101"}}}); err == nil {
		t.Fatal("want error for @101")
	}
}
