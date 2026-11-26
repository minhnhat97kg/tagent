package harnessusage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	for command, want := range map[string]float64{"echo 42": 42, "echo ' 42.5 '": 42.5, "printf '42%%'": 42, "echo 0": 0, "echo 100": 100, `echo "$P"`: 7} {
		if got := run(command, []string{"P=7"}, 5*time.Second); got == nil || *got != want {
			t.Errorf("%q: got %v, want %v", command, got, want)
		}
	}
	for _, command := range []string{"", "echo 101", "echo -1", "echo NaN", "echo 1e1", "echo 42 43", "echo 42; exit 1", "echo 42; printf '%100s' x", "printf '42%100s' ''", "no-such-command-here"} {
		if got := run(command, nil, 5*time.Second); got != nil {
			t.Errorf("%q: got %v, want an error", command, *got)
		}
	}
	start := time.Now()
	if got := run("sleep 30; echo 1", nil, 100*time.Millisecond); got != nil || time.Since(start) > 3*time.Second {
		t.Errorf("timeout: got %v after %s", got, time.Since(start))
	}
}

// writeHarnesses points the config dir at a harnesses.yml holding yml.
func writeHarnesses(t *testing.T, yml string) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	if err := os.MkdirAll(filepath.Join(config, "tagent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "tagent", "harnesses.yml"), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRowsAndRead(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	gate := filepath.Join(t.TempDir(), "gate")
	writeHarnesses(t, `harnesses:
  - name: Claude API
    env: {SECRET_PCT: "61"}
    usage:
      - label: month
        command: 'while [ ! -e "`+gate+`" ]; do sleep 0.02; done; echo "$SECRET_PCT"'
      - command: echo nope
  - name: pi
`)
	// The first command is still blocked, yet the call answers.
	rows := Rows()
	if len(rows) != 2 || rows[0].Harness != "Claude API" || rows[0].Label != "month" || rows[0].Used != nil || rows[0].Error {
		t.Fatalf("first rows: %+v", rows)
	}
	if r, ok := Read("Claude API"); !ok || len(r) != 0 {
		t.Fatalf("unread: %v %v", r, ok)
	}
	if _, ok := Read("pi"); ok {
		t.Fatal("pi has no usage commands")
	}
	if err := os.WriteFile(gate, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(4 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		rows = Rows()
		if rows[0].Used != nil && rows[1].Error {
			break
		}
		if time.Now().After(end) {
			t.Fatalf("rows: %+v", rows)
		}
	}
	if *rows[0].Used != 61 || rows[1].Label != "" {
		t.Fatalf("rows: %+v", rows)
	}
	if r, ok := Read("Claude API"); !ok || len(r) != 1 || r[0].Label != "month" || r[0].Used != 61 {
		t.Fatalf("read: %v %v", r, ok)
	}
	// Within the TTL the cached value is served without another run.
	if err := os.Remove(gate); err != nil {
		t.Fatal(err)
	}
	Rows()
	cache.Lock()
	defer cache.Unlock()
	for _, r := range cache.results {
		if r.running {
			t.Fatal("ran again within the TTL")
		}
	}
}
