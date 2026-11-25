package sidebar

import (
	tea "github.com/charmbracelet/bubbletea"
	"os/exec"
	"path/filepath"
	"strings"
	"tagent/internal/tmux"
	"testing"
)

func TestBrowserOwnedRowLabel(t *testing.T) {
	text := renderPaneText(Row{Base: "worker", SizedByWeb: true}, 80, DefaultTheme(), DefaultIcons(), false, 0)
	if !strings.Contains(text, "sized by web") {
		t.Fatalf("missing ownership label: %s", text)
	}
	text = renderPaneText(Row{Base: "worker"}, 80, DefaultTheme(), DefaultIcons(), false, 0)
	if strings.Contains(text, "sized by web") {
		t.Fatal("terminal-owned row mislabeled")
	}
}

// ─── Handoff key (ticket 42) ────────────────────────────────────────────────
//
// A row the web sizes is handed back to tmux by the f key. The tests below pin
// the three acceptance clauses: the key clears @agent_size_owner AND the row
// stops drawing "sized by web" as part of the keypress (not on the next 3 s
// refresh tick), the key does nothing on a tmux-owned row, and the key is
// advertised in the footer hints.

// pump runs cmd and feeds every message it produces back into the model, the
// way bubbletea's event loop does, so the test sees the state the user sees
// after the keypress. The periodic refresh tick is dropped: re-arming it would
// sleep 3 s and never settle.
func pump(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 50 {
			t.Fatal("command loop did not settle in 50 steps")
		}
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		msg := next()
		if _, tick := msg.(refreshTickMsg); tick {
			continue
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		if msg == nil {
			continue
		}
		updated, follow := m.Update(msg)
		m = updated.(Model)
		queue = append(queue, follow)
	}
	return m
}

// TestAgentKeyFHandoffClearsWebSizedRow is the ticket-42 reproduction on a real
// tmux server: on a row the web owns, the handoff key must empty
// @agent_size_owner AND stop drawing the row as "sized by web" as part of the
// keypress itself (acceptance 1) — not on the next 3 s refresh tick.
func TestAgentKeyFHandoffClearsWebSizedRow(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("no tmux on this machine, skipping integration test")
	}
	// Every tmux call, including the bare "tmux" the sidebar itself runs, must
	// land on a private server (same isolation as the tmux package's tests).
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	t.Cleanup(func() { exec.Command("tmux", "kill-server").Run() })

	dir := t.TempDir()
	agent := filepath.Join(dir, "pi")
	if err := exec.Command("ln", "-s", "/bin/sleep", agent).Run(); err != nil {
		t.Fatalf("symlink fake agent: %v", err)
	}
	out, err := exec.Command("tmux", "-f", "/dev/null", "new-session", "-d", "-c", dir, "-P", "-F", "#{pane_id}", agent+" 600").Output()
	if err != nil {
		t.Fatalf("tmux new-session: %v", err)
	}
	pane := strings.TrimSpace(string(out))

	tmux.SetPaneOption(pane, tmux.PaneAgent, "pi")
	tmux.SetPaneOption(pane, tmux.PaneStatusOpt, "running")
	tmux.SetPaneOption(pane, "@agent_size_owner", "web:abcdefgh")
	tmux.SetPaneOption(pane, "@agent_size_policy", "inherit")

	m := NewModel("")
	m.sessions = tmux.QuerySessions()
	m.sizeOwners = readSizeOwners(m.sessions)
	m.applyRows()
	idx, ok := paneIndexOf(m.rows, pane)
	if !ok {
		t.Fatalf("setup: pane %s is not in the Agents list: %+v", pane, m.rows)
	}
	m.cursor = idx
	if row, _ := paneRowAt(m.rows, m.cursor); !row.SizedByWeb {
		t.Fatalf("setup: row %s is not marked sized by web", pane)
	}

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if cmd == nil {
		t.Fatal("f on a web-sized row returned no command (no handoff)")
	}
	model := pump(t, next.(Model), cmd)

	if got := tmux.GetPaneOptionValue(pane, "@agent_size_owner"); got != "" {
		t.Errorf("@agent_size_owner = %q after f, want empty", got)
	}
	idx, ok = paneIndexOf(model.rows, pane)
	if !ok {
		t.Fatalf("row %s disappeared from the list: %+v", pane, model.rows)
	}
	if row, _ := paneRowAt(model.rows, idx); row.SizedByWeb {
		t.Error("row still renders \"sized by web\" after the handoff key (needs a refresh tick)")
	}
}

// TestAgentKeyFHandoffIgnoresTerminalOwnedRow is acceptance 2: the key does
// nothing on a row tmux already owns (@agent_size_owner empty). Today f hands
// off unconditionally, clearing/restoring the window sizing of a pane the web
// never touched.
func TestAgentKeyFHandoffIgnoresTerminalOwnedRow(t *testing.T) {
	fake := tmux.NewFake()
	defer tmux.SetRunner(fake)()
	fake.Set("", "show", "-t", "%1", "-pv", "@agent_size_owner")

	m := NewModel("%sidebar")
	m.rows = []Row{{PaneID: "%1"}}
	m.cursor = 0

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if cmd != nil {
		cmd()
	}
	for _, call := range fake.Calls() {
		switch call[0] {
		case "set-option", "set", "resize-window", "resize-pane":
			t.Errorf("f wrote tmux state on a tmux-owned row: %s", strings.Join(call, " "))
		}
	}
}

// TestAgentKeyFInFooterHints is the "in footer hints" half of acceptance 1:
// the handoff key must be discoverable without opening ? help.
func TestAgentKeyFInFooterHints(t *testing.T) {
	m := NewModel("%sidebar")
	m.width = 200
	for _, group := range hintKeys(m.footerHints()) {
		if group[0] == "f" {
			return
		}
	}
	t.Fatalf("footer hints do not advertise the handoff key: %s", m.footerHints())
}

func TestAgentKeyFHandoff(t *testing.T) {
	for _, dashboard := range []bool{false, true} {
		fake := tmux.NewFake()
		restore := tmux.SetRunner(fake)
		fake.Set("", "set-option", "-p", "-t", "%1", "@agent_size_owner", "")
		fake.Set("", "resize-window", "-A", "-t", "%1")
		m := NewModel("%sidebar")
		m.rows = pane3Rows()
		m.cursor = 0
		m.sizeOwners = map[string]bool{"%1": true} // f only acts on a row the web sizes
		m.dashboard = dashboard
		m.focus = 0
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
		if cmd == nil {
			restore()
			t.Fatal("f did not dispatch handoff")
		}
		cmd()
		found := false
		for _, c := range fake.Calls() {
			if strings.Join(c, " ") == "set-option -p -t %1 @agent_size_owner " {
				found = true
			}
		}
		restore()
		if !found {
			t.Fatal("handoff did not clear tmux owner")
		}
	}
}
