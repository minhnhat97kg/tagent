package sidebar

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestTmuxKey(t *testing.T) {
	for _, c := range []struct {
		k       tea.KeyMsg
		key     string
		literal bool
	}{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hi")}, "hi", true},
		{tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}, "Space", false},
		{tea.KeyMsg{Type: tea.KeyEnter}, "Enter", false},
		{tea.KeyMsg{Type: tea.KeyCtrlC}, "C-c", false},
		{tea.KeyMsg{Type: tea.KeyUp}, "Up", false},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x"), Alt: true}, "M-x", false},
		{tea.KeyMsg{Type: tea.KeyF5}, "F5", false},
		{tea.KeyMsg{Type: tea.KeyCtrlUp}, "C-Up", false},
		{tea.KeyMsg{Type: tea.KeyShiftTab}, "BTab", false},
		{tea.KeyMsg{Type: tea.KeyUp, Alt: true}, "M-Up", false},
	} {
		if key, lit := tmuxKey(c.k); key != c.key || lit != c.literal {
			t.Errorf("%v → %q,%v want %q,%v", c.k, key, lit, c.key, c.literal)
		}
	}
}

func TestWithCursor(t *testing.T) {
	got := withCursor("ab", 1, 10, true)
	if !strings.Contains(got, "\x1b[7mb\x1b[0m") {
		t.Errorf("cursor not on b: %q", got)
	}
	if got := withCursor("ab", 5, 10, false); !strings.Contains(got, "\x1b[4m \x1b[0m") {
		t.Errorf("cursor past the end must pad: %q", got)
	}
}

func TestLeaveKeys(t *testing.T) {
	for _, k := range []tea.KeyMsg{{Type: tea.KeyCtrlCloseBracket}, {Type: tea.KeyCtrlBackslash}} {
		if !isLeaveKey(k.String()) {
			t.Errorf("%q must leave the agent", k.String())
		}
	}
}

func TestHelpToggle(t *testing.T) {
	m := NewModel("%s")
	m.width, m.height = 44, 30
	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	hm := opened.(Model)
	if !hm.helpOpen || !strings.Contains(hm.View(), "Agents") {
		t.Fatal("? must open the shortcut list")
	}
	closed, _ := hm.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if closed.(Model).helpOpen {
		t.Fatal("any other key must close it")
	}
}

func TestStandaloneLayoutsAndEnterOnlyFromList(t *testing.T) {
	m := standaloneModel{w: 120, h: 30, left: Model{dashboard: true, theme: DefaultTheme()}}
	widths := []int{}
	for i := 0; i < layoutCount; i++ {
		m.layout = i
		rw, _ := m.rightSize()
		widths = append(widths, m.leftWidth(), rw)
		if m.layout != layoutDashboard && m.leftWidth()+rw+2+min(1, m.leftWidth()) != 120 { // +2: the frame's side borders
			t.Fatalf("layout %d: %d + %d", i, m.leftWidth(), rw)
		}
	}
	if widths[4] != 0 || widths[5] != 118 || widths[6] != 120 {
		t.Fatalf("agent/dashboard layouts: %v", widths)
	}
	m.layout = layoutSplit
	m.left.focus = 3 // Git panel: Enter is the dashboard's
	if m.listKeys() {
		t.Fatal("Enter taken from the Git panel")
	}
	m.left.focus = 0
	if !m.listKeys() {
		t.Fatal("agent list should own Enter")
	}
}

func TestStandaloneFollowFallsBackToShell(t *testing.T) {
	m := standaloneModel{shell: "%42", w: 120, h: 30}
	next, _ := m.follow(nil)
	if got := next.(standaloneModel).paneID; got != m.shell {
		t.Fatalf("pane = %q, want shell %q", got, m.shell)
	}
}

func TestStandaloneFrameLinesHaveEqualWidth(t *testing.T) {
	for _, focus := range []bool{false, true} {
		m := standaloneModel{w: 100, h: 24, paneID: "%1", focusRight: focus, screen: "hello\nworld", left: Model{dashboard: true, theme: DefaultTheme()}}
		for layout := 0; layout < layoutCount; layout++ {
			m.layout = layout
			if layout == layoutDashboard {
				continue
			}
			lines := strings.Split(ansi.Strip(m.View()), "\n")
			if len(lines) != m.h {
				t.Fatalf("layout %d: %d lines, want %d", layout, len(lines), m.h)
			}
			for i, l := range lines {
				if ansi.StringWidth(l) > m.w {
					t.Fatalf("layout %d line %d is %d wide", layout, i, ansi.StringWidth(l))
				}
			}
			if !strings.Contains(lines[m.h-1], "enter") && !strings.Contains(lines[m.h-1], "typing") {
				t.Fatalf("layout %d: no hint on the last line: %q", layout, lines[m.h-1])
			}
		}
	}
}
