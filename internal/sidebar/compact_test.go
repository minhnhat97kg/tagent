package sidebar

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// compactModel is a dashboard with a saved wide layout that is taller than its compact rows.
func compactModel(t *testing.T) (Model, string) {
	m := layoutModel(t)
	m.avatar = AvatarConfig{Character: "cat"}
	boxes := []paneBox{
		{ID: "agents", X: 0, Y: 0, W: 6, H: 14}, {ID: "usage", X: 0, Y: 14, W: 6, H: 12},
		{ID: "tasks", X: 6, Y: 0, W: 6, H: 26}, {ID: "workflows", X: 0, Y: 26, W: 6, H: 10},
		{ID: "comp", X: 6, Y: 26, W: 6, H: 8},
	}
	if err := saveLayout(true, boxes); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(layoutPath())
	if err != nil {
		t.Fatal(err)
	}
	m.layouts = loadLayouts()
	return m, string(data)
}

func TestCompactPacksAndRestoresTheStoredLayout(t *testing.T) {
	m, file := compactModel(t)
	before := m.dashGeometry().rects()
	m.compact = true
	got := m.dashGeometry().rects()
	for id, max := range compactRows {
		if r := got[id]; r.h > max || r.h == 0 {
			t.Fatalf("%s: h %d, want at most %d", id, r.h, max)
		}
	}
	for a, ra := range got {
		for b, rb := range got {
			if a < b && ra.h > 0 && rb.h > 0 && ra.x < rb.x+rb.w && rb.x < ra.x+ra.w && ra.y < rb.y+rb.h && rb.y < ra.y+ra.h {
				t.Fatalf("%s and %s overlap: %+v %+v", a, b, ra, rb)
			}
		}
	}
	if got["tasks"].y != 1 || got["agents"].y != 1 { // packed to the top, row 0 is the top bar
		t.Fatalf("not packed up: %+v", got)
	}
	if after, _ := os.ReadFile(layoutPath()); string(after) != file {
		t.Fatal("compact mode wrote layout.json")
	}
	m.compact = false
	if restored := m.dashGeometry().rects(); len(restored) != len(before) || restored["tasks"] != before["tasks"] || restored["comp"] != before["comp"] {
		t.Fatalf("turning compact off did not restore: %+v want %+v", restored, before)
	}
}

func TestCompactBlocksEditingAndDiscardsIt(t *testing.T) {
	m, file := compactModel(t)
	m.compact = true
	m = layoutPress(m, "M")
	if m.layoutEdit {
		t.Fatal("M opened the layout edit in compact mode")
	}
	m.compact = false
	m = layoutPress(m, "M", "l")
	m.compact = true
	next, _ := m.Update(RefreshMsg{Compact: true})
	m = next.(Model)
	if m.layoutEdit || m.layoutDirty || m.layoutDraft != nil {
		t.Fatalf("an edit survived compact mode: %+v", m.layoutDraft)
	}
	if after, _ := os.ReadFile(layoutPath()); string(after) != file {
		t.Fatal("discarding an edit wrote layout.json")
	}
}

func TestCompactRendersAtBothWidths(t *testing.T) {
	m, _ := compactModel(t)
	m.compact = true
	for _, w := range []int{40, 160} {
		m.width, m.height = w, 40
		lines := strings.Split(ansi.Strip(m.dashboardView()), "\n")
		if len(lines) != m.height {
			t.Fatalf("width %d: %d lines, want %d", w, len(lines), m.height)
		}
	}
}

// Clicks over the collapsed panes (rows, lanes, the job flow hit test) must not panic.
func TestCompactClicksDoNotPanic(t *testing.T) {
	m, _ := compactModel(t)
	m.compact = true
	m.width, m.height = 160, 40
	for y := 0; y < m.height; y++ {
		for x := 0; x < m.width; x += 2 {
			next, _ := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			m = next.(Model)
		}
	}
}
