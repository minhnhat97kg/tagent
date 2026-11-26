package sidebar

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLeftWidthKeysAndDragArePersisted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var m tea.Model = standaloneModel{w: 120, h: 30, left: Model{dashboard: true, theme: DefaultTheme()}}
	step := func(msg tea.Msg, want, saved int) {
		t.Helper()
		m, _ = m.Update(msg)
		if got, f := m.(standaloneModel).leftWidth(), loadLayouts().Left; got != want || f != saved {
			t.Fatalf("%v: width %d (saved %d), want %d (saved %d)", msg, got, f, want, saved)
		}
	}
	key := func(k string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)} }
	mouse := func(x int, a tea.MouseAction) tea.MouseMsg {
		return tea.MouseMsg{X: x, Y: 5, Button: tea.MouseButtonLeft, Action: a}
	}
	step(key(">"), 44, 44)
	step(key("<"), 40, 40)
	step(mouse(40, tea.MouseActionPress), 40, 40)
	step(mouse(60, tea.MouseActionMotion), 60, 40) // saved on release only
	step(mouse(60, tea.MouseActionRelease), 60, 60)
	step(key("="), 40, 0)
}

func TestLeftWidthAndPaneLayoutKeepEachOther(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := saveLeftWidth(55); err != nil {
		t.Fatal(err)
	}
	if err := saveLayout(true, []paneBox{{}}); err != nil {
		t.Fatal(err)
	}
	if f := loadLayouts(); f.Left != 55 || len(f.Wide) != 1 {
		t.Fatalf("saving the pane layout lost the width: %+v", f)
	}
	if err := saveLeftWidth(0); err != nil {
		t.Fatal(err)
	}
	if f := loadLayouts(); f.Left != 0 || len(f.Wide) != 1 {
		t.Fatalf("resetting the width lost the pane layout: %+v", f)
	}
}
