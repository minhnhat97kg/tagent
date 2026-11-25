package scrape

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"tagent/internal/tasks"
	"tagent/internal/tmux"
)

const stalledWorkflow = `name: feature
inputs:
  goal: {required: true}
jobs:
  dev: {status: fixing, agent: dev}
  review: {needs: dev, status: reviewing, agent: reviewer}
`

// stalledBoard is a board whose ticket sits in dev's status "fixing".
func stalledBoard(t *testing.T) (dir, id string) {
	t.Helper()
	t.Setenv("TMUX_AGENT_CONTEXT_DIR", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("TMUX_PANE", "")
	dir = t.TempDir()
	wf := filepath.Join(dir, ".tasks", "workflows")
	if err := os.MkdirAll(wf, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wf, "feature.yml"), []byte(stalledWorkflow), 0600); err != nil {
		t.Fatal(err)
	}
	if err := tasks.SaveBoardConfig(dir, []byte("default_workflow: feature\n")); err != nil {
		t.Fatal(err)
	}
	scope := tasks.Scope(dir)
	ticket, err := tasks.Add(scope, "goal", nil, tasks.MutationOptions{Workflow: "feature"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Move(scope, ticket.ID, "fixing", ""); err != nil {
		t.Fatal(err)
	}
	return dir, ticket.ID
}

func idlePane(id, dir string) []tmux.SessionInfo {
	return []tmux.SessionInfo{{
		SessionName: "tagent",
		Windows: []tmux.WindowInfo{{
			WindowID: "@1",
			Panes: []tmux.PaneInfo{{
				PaneID: "%1",
				Name:   "dev-" + id,
				Path:   dir,
				Status: tmux.StatusIdle,
				Agent:  tmux.AgentType("claude"),
			}},
		}},
	}}
}

// Ticket 77 / #75: a board agent that goes idle in its own status without
// moving its ticket (or asking via `task move blocked`) must get @pane_stuck
// and one notice. checkStuck used to watch running panes only.
func TestIdlePaneWithoutMoveIsFlaggedStuck(t *testing.T) {
	dir, id := stalledBoard(t)
	f := tmux.NewFake()
	defer tmux.SetRunner(f)()
	var notices []string
	defer func(old func(string, string)) { notifyStalled = old }(notifyStalled)
	notifyStalled = func(title, body string) { notices = append(notices, title) }

	sessions := idlePane(id, dir)
	w := newWatchdog()
	t0 := time.Date(2026, 11, 25, 10, 0, 0, 0, time.Local)
	w.checkStuck(sessions, t0)
	w.checkStuck(sessions, t0.Add(11*time.Minute))
	sessions[0].Windows[0].Panes[0].Stuck = "11"   // what tmux reports after the flag
	w.checkStuck(sessions, t0.Add(12*time.Minute)) // still stalled: no second notice

	want := []string{"set", "-t", "%1", "-p", tmux.PaneStuck, "11"}
	if !slices.ContainsFunc(f.Calls(), func(c []string) bool { return slices.Equal(c, want) }) {
		t.Fatalf("idle pane for 11m not flagged; tmux calls: %q", f.Calls())
	}
	if len(notices) != 1 {
		t.Fatalf("want one stalled notice, got %d: %q", len(notices), notices)
	}
}

// The flag clears once the ticket moves on: no stale "stuck" after a handoff.
func TestStalledFlagClearsWhenTicketMoves(t *testing.T) {
	dir, id := stalledBoard(t)
	f := tmux.NewFake()
	defer tmux.SetRunner(f)()
	defer func(old func(string, string)) { notifyStalled = old }(notifyStalled)
	notifyStalled = func(string, string) {}

	w := newWatchdog()
	t0 := time.Date(2026, 11, 25, 10, 0, 0, 0, time.Local)
	sessions := idlePane(id, dir)
	w.checkStuck(sessions, t0)
	w.checkStuck(sessions, t0.Add(11*time.Minute))

	if err := tasks.Move(tasks.Scope(dir), id, "reviewing", ""); err != nil {
		t.Fatal(err)
	}
	// The pane is still flagged as stuck from the watch before the move.
	flagged := idlePane(id, dir)
	flagged[0].Windows[0].Panes[0].Stuck = "11"
	w.checkStuck(flagged, t0.Add(12*time.Minute))
	want := []string{"set", "-t", "%1", "-p", "-u", tmux.PaneStuck}
	if slices.ContainsFunc(f.Calls(), func(c []string) bool { return slices.Equal(c, want) }) {
		return
	}
	t.Fatalf("stalled flag not cleared after ticket moved on; tmux calls: %q", f.Calls())
}
