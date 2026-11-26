package sidebar

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"tagent/internal/agents"
	"tagent/internal/workflow"
)

// hintKeys pulls the key group of every "key label" item out of a rendered hint
// line: "j/k" is one group, because at the edge of a list only one of them moves.
func hintKeys(line string) [][]string {
	line = strings.ReplaceAll(ansi.Strip(line), " · ", "  ")
	var keys [][]string
	for _, seg := range strings.Split(line, "  ") {
		f := strings.Fields(seg)
		if len(f) == 0 {
			continue
		}
		var group []string
		for _, k := range strings.Split(f[0], "/") {
			switch k {
			case "Tab":
				k = "tab"
			case "S-Tab":
				k = "shift+tab"
			case "Enter":
				k = "enter"
			case "Esc":
				k = "esc"
			case "↑↓":
				group = append(group, "up")
				k = "down"
			}
			if k != "" && k != "…" {
				group = append(group, k)
			}
		}
		if len(group) > 0 {
			keys = append(keys, group)
		}
	}
	return keys
}

func pressKey(m Model, key string) (string, tea.Cmd) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		msg = tea.KeyMsg{Type: tea.KeyShiftTab}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	}
	next, cmd := m.handleKey(msg)
	return fmt.Sprintf("%#v", next), cmd
}

func lastLines(s string, from int) string {
	l := strings.Split(s, "\n")
	return l[len(l)-from]
}

// Every key a footer hint names must do something in that context.
type hintCase struct {
	name  string
	model func() Model
	hint  func(Model) string
	noop  []string
}

func hintCases(t *testing.T) []hintCase {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // the layout keys save; keep that out of the real config
	flows := []workflow.Workflow{{Name: "research", File: "research.yml", Jobs: make([]workflow.Job, 1)}}
	ags := []agents.Agent{{Name: "planner", Harness: "pi", Source: "board"}}
	base := func() Model {
		m := boardTestModel()
		m.width, m.height, m.boardDir = 200, 40, t.TempDir()
		m.boardWorkflows = flows
		m.agents = ags
		return m
	}
	dash := func(focus int, mod func(*Model)) func() Model {
		return func() Model {
			m := base()
			m.maximized, m.focus, m.chartDays = false, focus, 7
			if mod != nil {
				mod(&m)
			}
			return m
		}
	}
	board := func(mod func(*Model)) func() Model {
		return func() Model { m := base(); mod(&m); return m }
	}
	dashHint := func(m Model) string { return lastLines(m.dashboardView(), 1) }
	// Documented no-ops: keys that need a live pane row, or content to scroll or browse
	// (the fixture has no workflow files and a one-screen ticket). f also needs a row the
	// web sizes, and the fixture has no web-owned pane.
	needsPane := []string{"enter", "i", "l", "up", "down", "f"}
	return []hintCase{
		{"dashboard agents", dash(0, nil), dashHint, needsPane},
		{"dashboard usage", dash(3, nil), dashHint, nil},
		{"dashboard tasks", dash(2, nil), dashHint, nil},
		{"dashboard workflows", dash(4, nil), dashHint, nil},
		{"dashboard layout", dash(0, func(m *Model) { *m = m.startLayoutEdit() }), dashHint, nil},
		{"dashboard agents tab", dash(4, func(m *Model) { m.agentsOpen = true }), dashHint, nil},
		{"board tickets", board(func(m *Model) {}), dashHint, nil},
		{"board workflows", board(func(m *Model) { m.focus = cardWorkflows }), dashHint, nil},
		{"board agents", board(func(m *Model) { m.focus, m.agentsOpen = cardWorkflows, true }), dashHint, nil},
		{"workflow view", board(func(m *Model) { m.boardFlow, m.flowWorkflow = true, "x.yml" }), dashHint, []string{"["}},
		{"board config", board(func(m *Model) { m.boardFlow, m.boardConfigView = true, true }), dashHint, []string{"j"}},
		{"drawer details", board(func(m *Model) { m.boardDrawer = true }), dashHint, []string{"j"}},
		{"drawer activity", board(func(m *Model) { m.boardDrawer, m.detailTab = true, detailTabActivity }), dashHint, []string{"j"}},
	}
}

// Every key a footer hint names must do something in that context.
func TestHintedKeysAreHandled(t *testing.T) {
	for _, c := range hintCases(t) {
		t.Run(c.name, func(t *testing.T) {
			m := c.model()
			keys := hintKeys(c.hint(m))
			if len(keys) < 2 {
				t.Fatalf("no hint keys found in %q", c.hint(m))
			}
			before := fmt.Sprintf("%#v", m)
			for _, group := range keys {
				if group[0] == "?" {
					continue // help is exercised by help_test.go
				}
				handled := slices.ContainsFunc(group, func(k string) bool { return slices.Contains(c.noop, k) })
				for _, k := range group {
					after, cmd := pressKey(m, k)
					handled = handled || after != before || cmd != nil
				}
				if !handled {
					t.Errorf("hinted key %q does nothing (hint %q)", group, ansi.Strip(c.hint(m)))
				}
			}
		})
	}
}

// paneKeys is every single key a pane lists (footer and ?), as handleKey sees it.
func paneKeys(m Model) map[string]bool {
	out := map[string]bool{}
	for _, h := range panes[m.focused()].Hints(m) {
		for _, f := range strings.FieldsFunc(h.Keys+" "+h.HelpKeys, func(r rune) bool { return r == ' ' || r == '/' }) {
			switch f {
			case "Tab":
				f = "tab"
			case "S-Tab":
				f = "shift+tab"
			case "↑↓":
				out["up"], f = true, "down"
			case "↑":
				f = "up"
			case "↓":
				f = "down"
			case "Enter":
				f = "enter"
			case "Esc":
				f = "esc"
			}
			if len([]rune(f)) == 1 || slices.Contains([]string{"up", "down", "left", "right", "enter", "esc", "tab", "shift+tab", "space", "backspace"}, f) {
				out[f] = true
			}
		}
	}
	return out
}

// A key only acts in its pane or is global: what other panes list does nothing here.
func TestOtherPanesKeysDoNothing(t *testing.T) {
	cases := hintCases(t)
	global := map[string]bool{"q": true, "ctrl+c": true, ",": true, "o": true, "?": true, "tab": true, "shift+tab": true, "B": true, "[": true, "]": true, "b": true, "esc": true, "enter": true, "up": true, "down": true}
	for _, c := range cases {
		m := c.model()
		own := paneKeys(m)
		before := fmt.Sprintf("%#v", m)
		for _, other := range cases {
			if panes[other.model().focused()].Title() == panes[m.focused()].Title() {
				continue
			}
			for k := range paneKeys(other.model()) {
				if own[k] || global[k] || k == " " {
					continue
				}
				if after, cmd := pressKey(m, k); after != before || cmd != nil {
					t.Errorf("%s: key %q (listed by %s) acted here", c.name, k, panes[other.model().focused()].Title())
				}
			}
		}
	}
}

func TestFocusedPane(t *testing.T) {
	for _, c := range []struct {
		m    Model
		want paneID
	}{
		{Model{}, paneAgents},
		{Model{dashboard: true}, paneAgents},
		{Model{dashboard: true, focus: cardUsage}, paneUsage},
		{Model{dashboard: true, focus: cardTasks}, paneTasks},
		{Model{dashboard: true, focus: cardWorkflows}, paneWorkflows},
		{Model{dashboard: true, focus: cardAgents, agentsOpen: true}, paneAgentDefs},
		{Model{dashboard: true, maximized: true, boardFlow: true}, paneWorkflowView},
		{Model{dashboard: true, maximized: true, boardFlow: true, boardConfigView: true}, paneBoardConfig},
		{Model{dashboard: true, maximized: true, boardDrawer: true}, paneTaskDetail},
	} {
		if got := c.m.focused(); got != c.want {
			t.Errorf("%+v: focused %d, want %d", c.m, got, c.want)
		}
	}
}
