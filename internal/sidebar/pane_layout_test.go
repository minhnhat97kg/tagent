package sidebar

import (
	"maps"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// layoutModel is the dashboard sample on a wide terminal, with its own config dir.
func layoutModel(t *testing.T) Model {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := dashSample()
	m.width, m.height = 120, 40
	return m
}

// layoutPress sends keys the way the terminal does.
func layoutPress(m Model, keys ...string) Model {
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "shift+tab":
			msg = tea.KeyMsg{Type: tea.KeyShiftTab}
		}
		next, _ := m.handleKey(msg)
		m = next.(Model)
	}
	return m
}

func TestLayoutEditSavesAndSurvivesRestart(t *testing.T) {
	m := layoutModel(t)
	before := m.dashGeometry().rects()
	m = layoutPress(m, "M", "l", "l", "L", "L", "J", "enter")
	if m.layoutEdit {
		t.Fatal("enter left the layout mode")
	}
	after := m.dashGeometry().rects()
	if maps.Equal(before, after) {
		t.Fatal("the layout did not change")
	}
	fresh := NewModel("") // a restart: the saved file is read again
	fresh.width, fresh.height = 120, 40
	if got := fresh.dashGeometry().rects(); !maps.Equal(got, after) {
		t.Fatalf("after restart %+v, want %+v", got, after)
	}
}

func TestLayoutEscDiscards(t *testing.T) {
	m := layoutModel(t)
	before := m.dashGeometry().rects()
	m = layoutPress(m, "M", "l", "L", "esc")
	if m.layoutEdit || !maps.Equal(m.dashGeometry().rects(), before) {
		t.Fatal("esc kept the edit")
	}
	if _, err := os.Stat(layoutPath()); !os.IsNotExist(err) {
		t.Fatalf("esc wrote %s: %v", layoutPath(), err)
	}
}

func TestLayoutZeroResetsToDefault(t *testing.T) {
	m := layoutModel(t)
	def := m.dashGeometry().rects()
	m = layoutPress(m, "M", "l", "l", "enter")
	m = layoutPress(m, "M", "0")
	if m.layouts.Wide != nil || !maps.Equal(m.dashGeometry().rects(), def) || loadLayouts().Wide != nil {
		t.Fatal("0 did not reset the layout")
	}
}

func TestLayoutWidthClassesAreIndependent(t *testing.T) {
	m := layoutModel(t)
	narrowDef := func() map[string]rect {
		n := m
		n.width = 60
		return n.dashGeometry().rects()
	}
	def := narrowDef()
	m = layoutPress(m, "M", "l", "l", "enter") // wide, 120 columns
	wide := m.dashGeometry().rects()
	m.width = 60
	if got := m.dashGeometry().rects(); !maps.Equal(got, def) {
		t.Fatal("a wide layout changed the narrow one")
	}
	m = layoutPress(m, "M", "j", "enter") // narrow, 60 columns
	m.width = 120
	if got := m.dashGeometry().rects(); !maps.Equal(got, wide) {
		t.Fatal("a narrow save changed the wide layout")
	}
}

// Moving a pane below the viewport scrolls the canvas to it; a click there hits the card.
func TestLayoutFollowsFocusAndHitsAfterScroll(t *testing.T) {
	m := layoutModel(t)
	m.height = 12
	m = layoutPress(m, "M")
	for range 15 {
		m = layoutPress(m, "j")
	}
	g := m.dashGeometry()
	if m.scrollY == 0 {
		t.Fatal("moving the pane down did not scroll the canvas")
	}
	if r := g.rects()["agents"]; r.y < 1+m.scrollY || r.y+r.h > 1+m.scrollY+g.bh {
		t.Fatalf("agents at rows %d..%d are outside the view %d..%d", r.y, r.y+r.h, 1+m.scrollY, 1+m.scrollY+g.bh)
	}
	m = layoutPress(m, "tab", "enter") // Usage, saved
	g = m.dashGeometry()
	ur := g.rects()["usage"]
	if ur.y < 1+m.scrollY || ur.y+ur.h > 1+m.scrollY+g.bh {
		t.Fatalf("usage is outside the view after tab")
	}
	m.focus = cardWorkflows
	click := tea.MouseMsg{X: ur.x + 1, Y: ur.y - m.scrollY + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := m.handleMouse(click)
	if got := next.(Model).focus; got != cardUsage {
		t.Fatalf("click on the scrolled usage card focused %d, want %d", got, cardUsage)
	}
}

// Every pane at the smallest size renders inside the width and the height.
func TestLayoutRendersAtMinimumSize(t *testing.T) {
	var boxes []paneBox
	for i, id := range layoutIDs {
		boxes = append(boxes, paneBox{ID: id, X: 0, Y: i * 3, W: 2, H: minPaneH})
	}
	for _, w := range []int{32, 60, 130} {
		m := dashSample()
		m.width, m.height = w, 40
		m.avatar = AvatarConfig{Character: "cat"}
		m.layouts = layoutFile{V: layoutFileVers, Narrow: boxes, Wide: boxes}
		lines := strings.Split(ansi.Strip(m.dashboardView()), "\n")
		if len(lines) != m.height {
			t.Fatalf("width %d: %d lines, want %d", w, len(lines), m.height)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > w {
				t.Fatalf("width %d: line %q is %d wide", w, line, ansi.StringWidth(line))
			}
		}
	}
}

// The Agents detail fills the pane unscrolled, so the wheel and clicks must not
// use the hidden canvas offset.
func TestAgentDetailIgnoresCanvasScroll(t *testing.T) {
	m := layoutModel(t)
	m.height = 12
	m = layoutPress(m, "M")
	for range 15 {
		m = layoutPress(m, "j")
	}
	m = layoutPress(m, "esc")
	m.dashboard, m.agentsOpen, m.agentDetail = true, true, true
	before := m.scrollY
	if before == 0 {
		t.Fatal("canvas is not scrolled, so the test proves nothing")
	}
	wheel := tea.MouseMsg{X: 0, Y: 1, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
	next, _ := m.handleMouse(wheel)
	if got := next.(Model).scrollY; got != before {
		t.Fatalf("wheel moved the hidden canvas: scrollY %d -> %d", before, got)
	}
}

// M is a dashboard key: on the board it does nothing (no invisible layout mode).
func TestLayoutNotFromBoardMode(t *testing.T) {
	m := layoutModel(t)
	m.boardMode = true
	m = layoutPress(m, "M")
	if m.layoutEdit {
		t.Fatal("M from board mode opened the layout mode")
	}
}

// enter without a change keeps the auto geometry: nothing is written.
func TestLayoutEnterWithoutChangeWritesNothing(t *testing.T) {
	m := layoutModel(t)
	def := m.dashGeometry().rects()
	m = layoutPress(m, "M", "enter")
	if _, err := os.Stat(layoutPath()); !os.IsNotExist(err) {
		t.Fatalf("enter without a change wrote %s: %v", layoutPath(), err)
	}
	m.width, m.height = 100, 30 // a resize still refits the default
	if maps.Equal(m.dashGeometry().rects(), def) {
		t.Fatal("the default did not refit after a resize")
	}
}

// A click on a card scrolled into view hits the card, not the one at the unscrolled spot.
func TestLayoutTasksClickAfterScroll(t *testing.T) {
	m := layoutModel(t)
	m.height = 12
	m.focus = cardTasks
	m = layoutPress(m, "M")
	for range 15 {
		m = layoutPress(m, "j")
	}
	m = layoutPress(m, "enter")
	m.focus = cardWorkflows
	r := m.dashGeometry().rects()["tasks"]
	click := tea.MouseMsg{X: r.x + 1, Y: r.y - m.scrollY + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	next, _ := m.handleMouse(click)
	if got := next.(Model).focus; got != cardTasks {
		t.Fatalf("click on the scrolled tasks card focused %d, want %d", got, cardTasks)
	}
}

// M opens the layout from every dashboard card.
func TestLayoutOpensFromEveryCard(t *testing.T) {
	for _, f := range []int{cardAgents, cardUsage, cardTasks, cardWorkflows} {
		m := layoutModel(t)
		m.focus = f
		if m = layoutPress(m, "M"); !m.layoutEdit {
			t.Errorf("M did not open the layout from card %d", f)
		}
	}
}
