package sidebar

import (
	"maps"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// dragEvent is a left-button mouse event at a screen cell.
func dragEvent(x, y int, action tea.MouseAction) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: action}
}

// mouseSend sends one mouse event the way the program does.
func mouseSend(m Model, msg tea.MouseMsg) Model {
	next, _ := m.handleMouse(msg)
	return next.(Model)
}

// noLayoutFile reports whether no layout has been written under this test's config dir.
func noLayoutFile(t *testing.T) bool {
	t.Helper()
	_, err := os.Stat(layoutPath())
	return os.IsNotExist(err)
}

// noOverlap reports whether two drawn panes share a cell.
func noOverlap(rects map[string]rect) bool {
	for a, ra := range rects {
		for b, rb := range rects {
			if a < b && ra.h > 0 && rb.h > 0 && ra.x < rb.x+rb.w && rb.x < ra.x+ra.w && ra.y < rb.y+rb.h && rb.y < ra.y+ra.h {
				return false
			}
		}
	}
	return true
}

func TestDragMoveByTitle(t *testing.T) {
	m := layoutModel(t)
	before := m.dashGeometry().rects()
	r := before["agents"]
	m = mouseSend(m, dragEvent(r.x+2, r.y, tea.MouseActionPress))
	m = mouseSend(m, dragEvent(r.x+2+10, r.y+2, tea.MouseActionMotion))
	m = mouseSend(m, dragEvent(r.x+2+10, r.y+2, tea.MouseActionRelease))
	after := m.dashGeometry().rects()
	if after["agents"] == r {
		t.Fatal("the pane did not move")
	}
	if !noOverlap(after) {
		t.Fatalf("panes overlap after the move: %+v", after)
	}
	fresh := NewModel("")
	fresh.width, fresh.height = 120, 40
	if got := fresh.dashGeometry().rects(); !maps.Equal(got, after) {
		t.Fatalf("after restart %+v, want %+v", got, after)
	}
}

func TestDragResizeByCorner(t *testing.T) {
	for _, dx := range []int{0, -1} { // both cells of the bottom-right corner
		m := layoutModel(t)
		r := m.dashGeometry().rects()["agents"]
		px := r.x + r.w - 1 + dx
		py := r.y + r.h - 1
		m = mouseSend(m, dragEvent(px, py, tea.MouseActionPress))
		m = mouseSend(m, dragEvent(px+12, py+2, tea.MouseActionMotion))
		m = mouseSend(m, dragEvent(px+12, py+2, tea.MouseActionRelease))
		got := m.dashGeometry().rects()["agents"]
		if got.w <= r.w || got.h <= r.h || got.x != r.x || got.y != r.y {
			t.Fatalf("corner at dx %d: got %+v from %+v, want wider and taller in place", dx, got, r)
		}
	}
}

func TestDragResizeClampsAtMinimum(t *testing.T) {
	m := layoutModel(t)
	r := m.dashGeometry().rects()["agents"]
	px, py := r.x+r.w-1, r.y+r.h-1
	m = mouseSend(m, dragEvent(px, py, tea.MouseActionPress))
	m = mouseSend(m, dragEvent(px-200, py-200, tea.MouseActionMotion))
	m = mouseSend(m, dragEvent(px-200, py-200, tea.MouseActionRelease))
	got := m.dashGeometry().rects()["agents"]
	if got.h != minPaneH || got.w <= 0 || got.w >= r.w {
		t.Fatalf("shrink went to %+v from %+v, want height %d and a narrow pane", got, r, minPaneH)
	}
}

func TestDragPressOnAgentsTabSwitchesWithoutSaving(t *testing.T) {
	m := layoutModel(t)
	ag := m.dashGeometry().agents
	_, spans := m.tabStrip([]string{"Running", "Definitions"}, b2i(m.agentsOpen), 3)
	x := ag.x + spans[1].x0 + 1
	m = mouseSend(m, dragEvent(x, ag.y, tea.MouseActionPress))
	m = mouseSend(m, dragEvent(x, ag.y, tea.MouseActionRelease))
	if !m.agentsOpen {
		t.Fatal("the Agents tab did not open")
	}
	if !noLayoutFile(t) {
		t.Fatal("a plain tab click saved a layout")
	}
}

func TestDragOutAndBackSavesNothing(t *testing.T) {
	m := layoutModel(t)
	before := m.dashGeometry().rects()
	r := before["agents"]
	m = mouseSend(m, dragEvent(r.x+2, r.y, tea.MouseActionPress))
	m = mouseSend(m, dragEvent(r.x+30, r.y+4, tea.MouseActionMotion))
	m = mouseSend(m, dragEvent(r.x+2, r.y, tea.MouseActionMotion))
	m = mouseSend(m, dragEvent(r.x+2, r.y, tea.MouseActionRelease))
	if m.layoutEdit || !noLayoutFile(t) {
		t.Fatal("a drag back to the start left an edit or wrote a layout")
	}
	if got := m.dashGeometry().rects(); !maps.Equal(got, before) {
		t.Fatalf("geometry %+v, want %+v", got, before)
	}
}

func TestDragWhileScrolledWithWheelMidDrag(t *testing.T) {
	m := layoutModel(t)
	r0 := m.dashGeometry().rects()["agents"]
	m = mouseSend(m, dragEvent(r0.x+2, r0.y, tea.MouseActionPress))
	m = mouseSend(m, dragEvent(r0.x+2, r0.y+20, tea.MouseActionMotion))
	m = mouseSend(m, dragEvent(r0.x+2, r0.y+20, tea.MouseActionRelease)) // a saved layout that reaches below a short viewport
	m.height = 20
	m.scrollY = 3
	g := m.dashGeometry()
	if _, sy := g.clampScroll(0, m.scrollY); sy != m.scrollY {
		t.Fatal("the sample does not scroll; the test needs a taller canvas")
	}
	r := g.rects()["agents"]
	sy := m.scrollY
	// Press the title on screen; the canvas is offset by sy.
	x, y := r.x+2, r.y-sy
	m = mouseSend(m, dragEvent(x, y, tea.MouseActionPress))
	m = mouseSend(m, tea.MouseMsg{X: 0, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	newSy := m.scrollY
	if newSy == sy {
		t.Fatal("the wheel did not scroll the canvas")
	}
	m = mouseSend(m, dragEvent(x, y+3, tea.MouseActionMotion))
	m = mouseSend(m, dragEvent(x, y+3, tea.MouseActionRelease))
	want := r.y + (y + 3 + newSy) - (y + sy)
	if got := m.dashGeometry().rects()["agents"]; got.y != want || got.x != r.x {
		t.Fatalf("agents at %+v, want row %d, column %d", got, want, r.x)
	}
}

func TestDragEscDiscards(t *testing.T) {
	m := layoutModel(t)
	before := m.dashGeometry().rects()
	r := before["agents"]
	m = mouseSend(m, dragEvent(r.x+2, r.y, tea.MouseActionPress))
	m = mouseSend(m, dragEvent(r.x+30, r.y+4, tea.MouseActionMotion))
	if !m.layoutEdit {
		t.Fatal("the drag did not change the layout")
	}
	m = layoutPress(m, "esc")
	if m.layoutEdit || m.drag != nil {
		t.Fatal("esc left the layout or the drag open")
	}
	m = mouseSend(m, dragEvent(r.x+30, r.y+4, tea.MouseActionRelease))
	if got := m.dashGeometry().rects(); !maps.Equal(got, before) || !noLayoutFile(t) {
		t.Fatalf("esc did not restore %+v (got %+v) or wrote a layout", before, got)
	}
}

func TestDragIgnoredInKeyboardLayoutMode(t *testing.T) {
	m := layoutModel(t)
	m = layoutPress(m, "M")
	before := m.dashGeometry().rects()
	r := before["agents"]
	m = mouseSend(m, dragEvent(r.x+2, r.y, tea.MouseActionPress))
	m = mouseSend(m, dragEvent(r.x+30, r.y+4, tea.MouseActionMotion))
	m = mouseSend(m, dragEvent(r.x+30, r.y+4, tea.MouseActionRelease))
	if got := m.dashGeometry().rects(); !maps.Equal(got, before) || !m.layoutEdit {
		t.Fatalf("mouse changed the keyboard layout: %+v", got)
	}
}

func TestStandaloneDragPastTheSeparatorSaves(t *testing.T) {
	m := standaloneModel{w: 120, h: 40, layout: layoutSplit, paneID: "%1", left: layoutModel(t)}
	updated, _ := m.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(standaloneModel)
	before := m.left.dashGeometry().rects()
	r := before["agents"]
	for _, ev := range []tea.MouseMsg{
		dragEvent(r.x+2, r.y, tea.MouseActionPress),
		dragEvent(m.rightX()+5, r.y+3, tea.MouseActionMotion), // past the separator: the right pane must not take it
		dragEvent(m.rightX()+5, r.y+3, tea.MouseActionRelease),
	} {
		updated, _ = m.update(ev)
		m = updated.(standaloneModel)
	}
	if m.left.drag != nil || noLayoutFile(t) || maps.Equal(m.left.dashGeometry().rects(), before) {
		t.Fatalf("drag across the separator did not finish and save: drag %+v", m.left.drag)
	}
	if m.focusRight {
		t.Fatal("the right pane took the drag")
	}
}

func TestStandaloneRightPressEndsStaleDrag(t *testing.T) {
	m := standaloneModel{w: 120, h: 40, layout: layoutSplit, paneID: "%1", left: layoutModel(t)}
	updated, _ := m.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(standaloneModel)
	before := m.left.dashGeometry().rects()
	r := before["agents"]
	for _, ev := range []tea.MouseMsg{
		dragEvent(r.x+2, r.y, tea.MouseActionPress), // release lost
		dragEvent(m.rightX()+5, r.y+3, tea.MouseActionPress),
		dragEvent(r.x+12, r.y+3, tea.MouseActionMotion), // no button held: must not move the pane
	} {
		updated, _ = m.update(ev)
		m = updated.(standaloneModel)
	}
	if m.left.drag != nil || !maps.Equal(m.left.dashGeometry().rects(), before) || !noLayoutFile(t) {
		t.Fatalf("a stale drag survived a right-pane press: drag %+v", m.left.drag)
	}
}

func TestStandaloneEnterSavesLayoutEdit(t *testing.T) {
	m := standaloneModel{w: 120, h: 40, layout: layoutSplit, left: layoutModel(t)}
	for _, k := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("M")}, {Type: tea.KeyRunes, Runes: []rune("l")}} {
		updated, _ := m.update(k)
		m = updated.(standaloneModel)
	}
	m.paneID = "%1" // an agent is selected: follow sets its pane, and enter would open it
	updated, _ := m.update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(standaloneModel)
	if m.left.layoutEdit || noLayoutFile(t) {
		t.Fatal("enter in the split view did not save the layout edit")
	}
}
