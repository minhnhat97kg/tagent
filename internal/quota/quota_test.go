package quota

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixtures(t *testing.T) {
	c := ParseClaude([]byte(`{"cachedUsageUtilization":{"fetchedAtMs":1,"utilization":{"five_hour":{"utilization":12,"resets_at":"2026-11-21T09:50:00.322693+00:00"},"seven_day":{"utilization":34}}}}`))
	if c.Five.Used == nil || *c.Five.Used != 12 || c.Week.Used == nil || *c.Week.Used != 34 {
		t.Fatalf("Claude: %+v", c)
	}
	d := ParseCodex([]byte(`{"id":2,"result":{"rateLimits":{"primary":{"usedPercent":23,"resetsAt":1767225600},"secondary":{"usedPercent":45}}}}`))
	if d.Five.Used == nil || *d.Five.Used != 23 || d.Week.Used == nil || *d.Week.Used != 45 {
		t.Fatalf("Codex: %+v", d)
	}
	if l := (Snapshot{c, d}).Lines(); len(l) != 1 || !strings.Contains(l[0], "12%·34%") || !strings.Contains(l[0], "23%·45%") {
		t.Fatalf("lines: %q", l)
	}
	if l := (Snapshot{}).Lines(); !strings.Contains(l[0], "n/a") {
		t.Fatalf("empty: %q", l)
	}
}

func TestOAuthFixtures(t *testing.T) {
	for _, tc := range []struct {
		body string
		five bool
	}{
		{`{"five_hour":{"utilization":16},"seven_day":{"utilization":45}}`, true},
		{`{"five_hour":null,"seven_day":{"utilization":45}}`, false},
	} {
		p, err := ParseClaudeOAuth([]byte(tc.body))
		if err != nil || (p.Five.Used != nil) != tc.five || p.Week.Used == nil || *p.Week.Used != 45 {
			t.Fatalf("OAuth: %+v %v", p, err)
		}
	}
	p := ParseClaude([]byte(`{"cachedUsageUtilization":{"fetchedAtMs":1,"utilization":{"five_hour":{"utilization":16}}}}`))
	if !p.Stale || !strings.Contains((Snapshot{Claude: p}).Lines()[0], "(stale)") {
		t.Fatalf("fallback: %+v", p)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("error")), Header: make(http.Header)}, nil
}
func TestClaudeFallbackOnAPIError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir) // the shared API cache lives under the user cache dir
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("CLAUDE_CONFIG_JSON", filepath.Join(dir, "cache.json"))
	t.Setenv("PATH", dir)
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"fixture"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cache.json"), []byte(`{"cachedUsageUtilization":{"fetchedAtMs":1,"utilization":{"five_hour":{"utilization":16}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	old := http.DefaultTransport
	http.DefaultTransport = failingTransport{}
	defer func() { http.DefaultTransport = old }()
	p := claude()
	if !p.Stale || p.Five.Used == nil || *p.Five.Used != 16 {
		t.Fatalf("fallback: %+v", p)
	}
}

func TestClaudeAPISharedCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	calls := 0
	old := http.DefaultTransport
	http.DefaultTransport = roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"five_hour":{"utilization":7},"seven_day":{"utilization":8}}`)), Header: make(http.Header)}, nil
	})
	defer func() { http.DefaultTransport = old }()
	for i := 0; i < 3; i++ {
		p, ok := claudeAPI("fixture")
		if !ok || p.Stale || *p.Five.Used != 7 {
			t.Fatalf("call %d: %+v %v", i, p, ok)
		}
	}
	if calls != 1 {
		t.Fatalf("endpoint called %d times, want 1 (shared cache)", calls)
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
