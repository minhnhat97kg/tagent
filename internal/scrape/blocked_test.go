package scrape

import (
	"testing"
	"time"

	"tagent/internal/board"
	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

// A blocked ticket alerts once after the limit, not again on later checks, and
// alerts again only for a new blocked spell.
func TestBlockedAlertsOncePerSpell(t *testing.T) {
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	defer tmux.SetRunner(tmux.NewFake())()
	var sent []string
	old := sendBlocked
	sendBlocked = func(title, body string) { sent = append(sent, body) }
	defer func() { sendBlocked = old }()

	dir := t.TempDir()
	if err := board.Register(dir); err != nil {
		t.Fatal(err)
	}
	scope := tasks.Scope(dir)
	tk, err := tasks.Add(scope, "which API?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = tasks.Move(scope, tk.ID, "blocked", "pick one"); err != nil {
		t.Fatal(err)
	}
	since := time.Now()

	checkBlocked(since.Add(time.Hour)) // under the 24h limit
	if len(sent) != 0 {
		t.Fatalf("alerted before the limit: %v", sent)
	}
	checkBlocked(since.Add(25 * time.Hour))
	checkBlocked(since.Add(26 * time.Hour)) // a later check, or a restart, reads the record
	if len(sent) != 1 {
		t.Fatalf("want one alert for the spell, got %d: %v", len(sent), sent)
	}

	if err = tasks.Move(scope, tk.ID, "todo", ""); err != nil {
		t.Fatal(err)
	}
	if err = tasks.Move(scope, tk.ID, "blocked", "still stuck"); err != nil {
		t.Fatal(err)
	}
	checkBlocked(time.Now().Add(25 * time.Hour)) // a new spell alerts again
	if len(sent) != 2 {
		t.Fatalf("want a second alert for the new spell, got %d: %v", len(sent), sent)
	}
}
