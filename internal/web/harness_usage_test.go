package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tagent/internal/metrics"
	"tagent/internal/quota"
	"tagent/internal/tmux"
)

func TestRunUsage(t *testing.T) {
	for command, want := range map[string]float64{"echo 42": 42, "echo ' 42.5 '": 42.5, "printf '42%%'": 42, "echo 0": 0, "echo 100": 100, `echo "$P"`: 7} {
		if got := runUsage(command, []string{"P=7"}, 5*time.Second); got == nil || *got != want {
			t.Errorf("%q: got %v, want %v", command, got, want)
		}
	}
	for _, command := range []string{"", "echo 101", "echo -1", "echo NaN", "echo 1e1", "echo 42 43", "echo 42; exit 1", "echo 42; printf '%100s' x", "printf '42%100s' ''", "no-such-command-here"} {
		if got := runUsage(command, nil, 5*time.Second); got != nil {
			t.Errorf("%q: got %v, want an error", command, *got)
		}
	}
	start := time.Now()
	if got := runUsage("sleep 30; echo 1", nil, 100*time.Millisecond); got != nil || time.Since(start) > 3*time.Second {
		t.Errorf("timeout: got %v after %s", got, time.Since(start))
	}
}

func TestOverviewHarnessUsage(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	gate := filepath.Join(t.TempDir(), "gate")
	yml := `harnesses:
  - name: Claude API
    env: {SECRET_PCT: "61"}
    usage:
      - label: month
        command: 'while [ ! -e "` + gate + `" ]; do sleep 0.02; done; echo "$SECRET_PCT"'
      - command: echo nope
  - name: pi
`
	if err := os.MkdirAll(filepath.Join(config, "tagent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "tagent", "harnesses.yml"), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &server{token: "secret", dir: t.TempDir(), deps: apiDeps{sessions: func() []tmux.SessionInfo { return nil }, refresh: func(string, []tmux.SessionInfo) metrics.State { return metrics.State{} }, quota: func() quota.Snapshot { return quota.Snapshot{} }}}
	get := func() string { return request(s, "GET", "/api/overview", "", true).Body.String() }
	// The first command is still blocked, yet the request answers.
	body := get()
	if !strings.Contains(body, `{"harness":"Claude API","label":"month","usedPercent":null,"error":false}`) {
		t.Fatal(body)
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(4 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		body = get()
		if strings.Contains(body, `"label":"month","usedPercent":61,"error":false`) && strings.Contains(body, `"label":"","usedPercent":null,"error":true`) {
			break
		}
		if time.Now().After(end) {
			t.Fatal(body)
		}
	}
	if strings.Contains(body, "SECRET_PCT") || strings.Contains(body, "echo") {
		t.Fatalf("command or env leaked: %s", body)
	}
	// Within the TTL the cached value is served without another run.
	if err := os.Remove(gate); err != nil {
		t.Fatal(err)
	}
	get()
	s.usage.Lock()
	defer s.usage.Unlock()
	for _, r := range s.usage.results {
		if r.running {
			t.Fatal("ran again within the TTL")
		}
	}
}
