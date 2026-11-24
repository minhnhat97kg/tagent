package sidebar

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// newAgentTestModel is a dashboard with one repo agent ("reviewer") in a temp
// repo, focused on the Agents card — the context the N form opens from.
func newAgentTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "xdg"))
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".tasks", "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	agent := "---\ndescription: reviews\n---\nReview carefully.\n"
	if err := os.WriteFile(filepath.Join(dir, ".tasks", "agents", "reviewer.md"), []byte(agent), 0644); err != nil {
		t.Fatal(err)
	}
	m := NewModel("%10")
	m.EnableDashboard()
	m.width, m.height = 120, 40
	m.focus = cardAgents
	m.repoDir = dir
	return m
}

func openNewAgentForm(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("N")})
	m = next.(Model)
	if m.taskForm != "new-agent" {
		t.Fatalf("N did not open the new-agent form: %q", m.taskForm)
	}
	// The configured harness comes from tmux; pin the row for a stable test.
	m.newAgent.harness = 0
	m.newAgent.roles = nil // free-text fallback; TestNewAgentFormPicksRepoAgent covers the picker
	return m
}

// The repo-agent row lists the repo's agents and space cycles through them.
func TestNewAgentFormPicksRepoAgent(t *testing.T) {
	m := openNewAgentForm(t, newAgentTestModel(t))
	m.newAgent.roles = m.newAgentRoles()
	if !slices.Contains(m.newAgent.roles, "reviewer") {
		t.Fatalf("roles: %v", m.newAgent.roles)
	}
	next, _ := m.handleTaskForm(tea.KeyMsg{Type: tea.KeySpace})
	if got := next.(Model).newAgent.role; got == "" {
		t.Fatalf("space did not pick a repo agent")
	}
}

// TestNewAgentFormOffersWebFields reproduces ticket 43: the key that starts a
// new agent ("N" in the Agents pane) opens a form with the fields the web's
// New agent composer has, so the TUI no longer needs `tagent new` by hand.
func TestNewAgentFormOffersWebFields(t *testing.T) {
	m := openNewAgentForm(t, newAgentTestModel(t))

	// The web composer's fields: repo agent (role), harness, model, worktree.
	view := strings.ToLower(ansi.Strip(m.taskFormView()))
	for _, want := range []string{"repo-agent", "harness", "model", "worktree"} {
		if !strings.Contains(view, want) {
			t.Errorf("new-agent form has no %q field:\n%s", want, ansi.Strip(m.taskFormView()))
		}
	}

	// Typing a name reaches the repo-agent field and shows on screen.
	next, _ := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("reviewer")})
	m = next.(Model)
	if !strings.Contains(m.taskInput, "reviewer") {
		t.Errorf("repo-agent name not typed into the form: %q", m.taskInput)
	}
	if !strings.Contains(ansi.Strip(m.taskFormView()), "reviewer") {
		t.Errorf("form does not show the typed repo agent:\n%s", ansi.Strip(m.taskFormView()))
	}

	// Esc cancels without starting anything.
	if next, cmd := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEsc}); next.(Model).taskForm != "" || cmd != nil {
		t.Errorf("esc did not cancel cleanly: form %q cmd %v", next.(Model).taskForm, cmd)
	}
}

// TestNewAgentFormSubmitsWebFlags walks the form and pins the command line:
// the flags the web's New agent action sends to `tagent new`.
func TestNewAgentFormSubmitsWebFlags(t *testing.T) {
	m := openNewAgentForm(t, newAgentTestModel(t))
	press := func(key tea.KeyMsg) {
		t.Helper()
		next, _ := m.handleTaskForm(key)
		m = next.(Model)
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("reviewer")}) // repo-agent
	press(tea.KeyMsg{Type: tea.KeyEnter})                            // harness row
	if m.newAgent.field != newAgentHarness {
		t.Fatalf("enter did not move to the harness row: %d", m.newAgent.field)
	}
	press(tea.KeyMsg{Type: tea.KeySpace}) // claude → codex
	press(tea.KeyMsg{Type: tea.KeyEnter})
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("opus-4")}) // model
	press(tea.KeyMsg{Type: tea.KeyEnter})                          // worktree row
	press(tea.KeyMsg{Type: tea.KeySpace})                          // worktree on

	f := m.newAgent
	if f.role != "reviewer" || f.harnessName() != "codex" || f.model != "opus-4" || !f.worktree {
		t.Fatalf("form state: %+v", f)
	}
	want := []string{"new", "--no-wait", "--cwd", m.repoDir, "--repo-agent", "reviewer", "--agent", "codex", "--model", "opus-4", "--worktree"}
	if got := newAgentArgs(m.newAgentCwd(), f); !slices.Equal(got, want) {
		t.Fatalf("args:\n got %q\nwant %q", got, want)
	}

	// ctrl+s starts from any row: the form closes and a start command is
	// returned (not run here — it would exec the test binary).
	next, cmd := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(Model)
	if m.taskForm != "" || m.taskInput != "" || cmd == nil {
		t.Fatalf("submit did not close the form and start: %q %v", m.taskForm, cmd)
	}
}

// TestNewAgentFormPlainAgentAndInlineErrors: an empty repo agent starts a plain
// agent on the chosen harness (the web's role is optional too, and the old N
// picker started plain agents); Esc and a bad repo agent leave no side effect
// and the reason shows inside the form.
func TestNewAgentFormPlainAgentAndInlineErrors(t *testing.T) {
	m := openNewAgentForm(t, newAgentTestModel(t))

	// No repo agent: --repo-agent is omitted, so a plain harness agent starts.
	want := []string{"new", "--no-wait", "--cwd", m.repoDir, "--agent", "claude"}
	if got := newAgentArgs(m.newAgentCwd(), m.newAgent); !slices.Equal(got, want) {
		t.Fatalf("plain agent args:\n got %q\nwant %q", got, want)
	}
	next, cmd := m.handleTaskForm(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(Model)
	if m.taskForm != "" || m.taskInput != "" || cmd == nil {
		t.Fatalf("empty repo agent did not start a plain agent: %q %v", m.taskForm, cmd)
	}

	// An unknown repo agent is rejected with the list the repo has.
	m = openNewAgentForm(t, m)
	next, _ = m.handleTaskForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ghost")})
	m = next.(Model)
	next, cmd = m.handleTaskForm(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = next.(Model)
	if cmd != nil || !strings.Contains(m.taskNotice, "unknown repo agent") || !strings.Contains(m.taskNotice, "reviewer") {
		t.Fatalf("unknown repo agent: %q %v", m.taskNotice, cmd)
	}
	if !strings.Contains(ansi.Strip(m.taskFormView()), "unknown repo agent") {
		t.Fatalf("error not shown in the form:\n%s", ansi.Strip(m.taskFormView()))
	}
	if m.newAgent.field != newAgentRole {
		t.Fatalf("error did not return to the repo-agent row: %d", m.newAgent.field)
	}

	// Typing clears the error.
	next, _ = m.handleTaskForm(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("er")})
	m = next.(Model)
	if m.taskNotice != "" {
		t.Fatalf("typing did not clear the error: %q", m.taskNotice)
	}

	// Esc still cancels from a form holding an error.
	next, cmd = m.handleTaskForm(tea.KeyMsg{Type: tea.KeyEsc})
	if next.(Model).taskForm != "" || cmd != nil {
		t.Fatalf("esc did not cancel: %q %v", next.(Model).taskForm, cmd)
	}
}

// TestNewAgentKeyInHintsAndHelp: the key is named in the footer and in ?.
func TestNewAgentKeyInHintsAndHelp(t *testing.T) {
	m := newAgentTestModel(t)
	footer := ansi.Strip(m.footerHints())
	if !strings.Contains(footer, "N") || !strings.Contains(footer, "new agent") {
		t.Fatalf("footer does not name the new-agent key: %q", footer)
	}
	if !slices.ContainsFunc(m.paneSection(paneAgents).keys, func(kv [2]string) bool {
		return kv[0] == "N" && kv[1] == "new agent"
	}) {
		t.Fatalf("? help has no new-agent row: %v", m.paneSection(paneAgents).keys)
	}
}
