package sidebar

import (
	tea "github.com/charmbracelet/bubbletea"
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

func TestAgentKeyFHandoff(t *testing.T) {
	for _, dashboard := range []bool{false, true} {
		fake := tmux.NewFake()
		restore := tmux.SetRunner(fake)
		fake.Set("", "set-option", "-p", "-t", "%1", "@agent_size_owner", "")
		fake.Set("", "resize-window", "-A", "-t", "%1")
		m := NewModel("%sidebar")
		m.rows = pane3Rows()
		m.cursor = 0
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
