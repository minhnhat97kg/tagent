package schedule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tagent/internal/board"
)

func TestNext(t *testing.T) {
	at := func(s string) Entry { return Entry{At: s} }
	every := func(d time.Duration) Entry { return Entry{Every: d} }
	day := func(h, m int) time.Time { return time.Date(2026, 11, 26, h, m, 0, 0, time.UTC) }
	cases := []struct {
		name string
		e    Entry
		last time.Time
		want time.Time
	}{
		{"at later today", at("08:00"), day(7, 0), day(8, 0)},
		{"at exactly now is strictly after", at("08:00"), day(8, 0), day(8, 0).AddDate(0, 0, 1)},
		{"at already passed rolls to tomorrow", at("08:00"), day(9, 30), day(8, 0).AddDate(0, 0, 1)},
		{"every adds the interval", every(30 * time.Minute), day(10, 0), day(10, 30)},
		{"every from a missed run is still one step ahead", every(time.Hour), day(7, 0), day(8, 0)},
	}
	for _, c := range cases {
		if got := c.e.Next(c.last); !got.Equal(c.want) {
			t.Errorf("%s: next %v, want %v", c.name, got, c.want)
		}
	}
}

// A missed run fires once, claimed before it runs; a second check (a restart,
// or the next minute) does not fire the same slot again.
func TestCheckFiresOnceAcrossMissedRunsAndRestart(t *testing.T) {
	root := t.TempDir()
	file := LocalFile(root)
	os.MkdirAll(filepath.Dir(file), 0700)
	os.WriteFile(file, []byte("- name: beat\n  every: 1m\n  send: {agent: a, prompt: p}\n"), 0600)
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	var fired []string
	fire := func(e Entry) (string, error) { fired = append(fired, e.Name); return "", nil }

	// First sight arms and does not fire.
	if err := Check(file, root, now, fire); err != nil || len(fired) != 0 {
		t.Fatalf("first sight: err %v fired %v", err, fired)
	}
	// Machine off for 3 hours, then three checks in a row (the last one a restart): one fire.
	later := now.Add(3 * time.Hour)
	for _, at := range []time.Time{later, later.Add(10 * time.Second), later.Add(20 * time.Second)} {
		if err := Check(file, root, at, fire); err != nil {
			t.Fatal(err)
		}
	}
	if len(fired) != 1 {
		t.Fatalf("missed run fired %d times, want 1: %v", len(fired), fired)
	}
}

func TestParseValidation(t *testing.T) {
	const ok = "- name: ok\n  every: 5m\n  send: {agent: a, prompt: p}\n"
	cases := []struct {
		name, bad, msg string
		global         bool
	}{
		{"no name", "- every: 5m\n  send: {agent: a, prompt: p}\n", "name is required", false},
		{"at and every", "- name: x\n  at: \"08:00\"\n  every: 5m\n  send: {agent: a, prompt: p}\n", "exactly one of at or every", false},
		{"bad time", "- name: x\n  at: \"25:00\"\n  send: {agent: a, prompt: p}\n", "at must be HH:MM", false},
		{"short interval", "- name: x\n  every: 30s\n  send: {agent: a, prompt: p}\n", "at least 1m", false},
		{"workflow and send", "- name: x\n  every: 5m\n  workflow: bugfix\n  send: {agent: a, prompt: p}\n", "exactly one of workflow or send", false},
		{"unknown key", "- name: x\n  every: 5m\n  cron: \"* * * * *\"\n  send: {agent: a, prompt: p}\n", "unknown key cron", false},
		{"local dir", "- name: x\n  every: 5m\n  dir: /tmp\n  send: {agent: a, prompt: p}\n", "dir is not allowed", false},
		{"global workflow needs dir", "- name: x\n  every: 5m\n  workflow: bugfix\n", "needs dir", true},
		{"send without prompt", "- name: x\n  every: 5m\n  send: {agent: a}\n", "send needs agent and prompt", false},
		{"title on send", "- name: x\n  every: 5m\n  title: t\n  send: {agent: a, prompt: p}\n", "title and inputs go with workflow", false},
	}
	for _, c := range cases {
		entries, err := Parse([]byte(c.bad+ok), "f.yml", c.global)
		if err != nil || len(entries) != 2 {
			t.Fatalf("%s: err %v entries %d", c.name, err, len(entries))
		}
		bad, good := entries[0], entries[1]
		if bad.Err == nil || bad.Enabled || !strings.Contains(bad.Err.Error(), c.msg) || !strings.HasPrefix(bad.Err.Error(), "f.yml:1: ") {
			t.Errorf("%s: err %v enabled %v, want f.yml:1 with %q", c.name, bad.Err, bad.Enabled, c.msg)
		}
		if good.Err != nil || !good.Enabled || good.Name != "ok" {
			t.Errorf("%s: sibling broken: %+v", c.name, good.Err)
		}
	}
	dup, err := Parse([]byte(ok+ok), "f.yml", false)
	if err != nil || dup[0].Err != nil || dup[1].Err == nil || !strings.Contains(dup[1].Err.Error(), "duplicate name ok") {
		t.Errorf("duplicate name: %v %v", dup, err)
	}
	if _, err := Parse([]byte("name: x\n"), "f.yml", false); err == nil {
		t.Error("a mapping instead of a list must fail the file")
	}
}

// A global workflow never registers its repo: an unregistered dir is refused,
// and no ticket is made there.
func TestFiresRefusesUnregisteredRepo(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	_, err := Fires(Entry{Name: "x", Workflow: "bugfix", Dir: dir, Global: true})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("want a not-registered refusal, got %v", err)
	}
	if len(board.Registered()) != 0 {
		t.Fatal("firing registered the repo")
	}
}

func TestTrustRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if len(Trusted()) != 0 {
		t.Fatal("a missing trust list must trust nothing")
	}
	if err := Trust("/a", true); err != nil {
		t.Fatal(err)
	}
	if err := Trust("/a", true); err != nil { // idempotent
		t.Fatal(err)
	}
	if err := Trust("/b", true); err != nil {
		t.Fatal(err)
	}
	if err := Trust("/a", false); err != nil {
		t.Fatal(err)
	}
	if got := Trusted(); len(got) != 1 || got[0] != "/b" {
		t.Fatalf("trusted %v", got)
	}
}
