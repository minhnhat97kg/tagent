package sidebar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"tagent/internal/settings"
)

// The free-form pane layout (see .tasks/plans/105.md). A pane is a box on a
// canvas: x and w are units, 12 per viewport width (up to 24, so the canvas can
// overflow to the right); y and h are rows. Boxes never overlap: the moved box
// stays and the others move down.
const (
	layoutUnits    = 12
	maxLayoutUnit  = 24
	maxLayoutRow   = 500
	minPaneH       = 3 // the smallest pane in rows, borders included
	defaultCompH   = 8 // the companion's rows while the avatar is off: its box is stored, not drawn
	layoutFileVers = 1
)

// layoutIDs are the built-in panes, in Tab order; comp is the companion (avatar).
var layoutIDs = []string{"agents", "usage", "tasks", "workflows", "comp"}

type paneBox struct {
	ID string `json:"id"`
	X  int    `json:"x"`
	Y  int    `json:"y"`
	W  int    `json:"w"`
	H  int    `json:"h"`
}

func normalizeBox(b paneBox, minH int) paneBox {
	b.W = max(2, min(maxLayoutUnit, b.W))
	b.X = max(0, min(maxLayoutUnit-b.W, b.X))
	b.Y = max(0, min(maxLayoutRow, b.Y))
	b.H = max(minH, min(maxLayoutRow, b.H))
	return b
}

func (b paneBox) overlaps(o paneBox) bool {
	return b.X < o.X+o.W && o.X < b.X+b.W && b.Y < o.Y+o.H && o.Y < b.Y+b.H
}

// placeBoxes keeps the moved box where it is and moves every other box down, in
// (y, x) order, to the first row where it overlaps no placed box. Output keeps
// the input order.
func placeBoxes(boxes []paneBox, moved string) []paneBox {
	out := slices.Clone(boxes)
	var placed []paneBox
	var rest []int
	for i, b := range out {
		if b.ID == moved {
			placed = append(placed, b)
		} else {
			rest = append(rest, i)
		}
	}
	slices.SortStableFunc(rest, func(a, c int) int {
		if d := out[a].Y - out[c].Y; d != 0 {
			return d
		}
		return out[a].X - out[c].X
	})
	for _, i := range rest {
		b := out[i]
		for slices.ContainsFunc(placed, b.overlaps) {
			b.Y++
		}
		out[i] = b
		placed = append(placed, b)
	}
	return out
}

// freeRow is the first row, above low, where b overlaps none of placed; low when
// there is none.
func freeRow(placed []paneBox, b paneBox, low int) int {
	for y := range low {
		b.Y = y
		if !slices.ContainsFunc(placed, b.overlaps) {
			return y
		}
	}
	return low
}

// mergeBoxes is a saved layout over the defaults: saved boxes keep their place
// (clamped), a repeated id keeps its first box, missing defaults take the first
// free row above the lowest box, then placeBoxes runs with moved. The defaults are the built-in
// ids; any other saved id (custom ext:* panes, unknown ones) is appended
// unchanged, so it is kept in storage and never pushes a drawn pane down.
func mergeBoxes(saved, defaults []paneBox, minH int, moved string) []paneBox {
	known := map[string]bool{}
	for _, d := range defaults {
		known[d.ID] = true
	}
	var out, unknown []paneBox
	seen := map[string]bool{}
	add := func(b paneBox) {
		if seen[b.ID] {
			return
		}
		seen[b.ID] = true
		if known[b.ID] {
			out = append(out, normalizeBox(b, minH))
		} else {
			unknown = append(unknown, b)
		}
	}
	for _, b := range saved {
		add(b)
	}
	low := 0
	for _, b := range out {
		low = max(low, b.Y+b.H)
	}
	for _, d := range defaults {
		if !seen[d.ID] {
			d.Y = freeRow(out, d, low)
			add(d)
		}
	}
	return append(placeBoxes(out, moved), unknown...)
}

// layoutFile is layout.json beside config.conf: one layout per width class.
type layoutFile struct {
	V      int       `json:"v"`
	Narrow []paneBox `json:"narrow,omitempty"`
	Wide   []paneBox `json:"wide,omitempty"`
}

func (f layoutFile) boxes(wide bool) []paneBox {
	if wide {
		return f.Wide
	}
	return f.Narrow
}

func (f *layoutFile) set(wide bool, boxes []paneBox) {
	if wide {
		f.Wide = boxes
	} else {
		f.Narrow = boxes
	}
}

func layoutPath() string { return filepath.Join(filepath.Dir(settings.Path()), "layout.json") }

// loadLayouts reads the saved layouts. A missing, corrupt or unknown-version
// file means no saved layout.
func loadLayouts() layoutFile {
	var f layoutFile
	data, err := os.ReadFile(layoutPath())
	if err != nil || json.Unmarshal(data, &f) != nil || f.V != layoutFileVers {
		return layoutFile{}
	}
	return f
}

// saveLayout stores one width class (nil deletes it) and keeps the other. It
// re-reads the file first, so a save from another sidebar is not lost.
func saveLayout(wide bool, boxes []paneBox) error {
	f := loadLayouts()
	f.V = layoutFileVers
	f.set(wide, boxes)
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	path := layoutPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// unitsOf is px as canvas units for a viewport of width columns (rounded).
func unitsOf(px, width int) int { return (px*layoutUnits*2 + width) / (2 * width) }

// boxesFrom is a computed geometry as boxes: the defaults that a saved layout merges onto.
func boxesFrom(g dashGeom) []paneBox {
	var out []paneBox
	rects := g.rects()
	for _, id := range layoutIDs {
		r := rects[id]
		if r.h == 0 {
			continue
		}
		x := unitsOf(r.x, g.bw)
		out = append(out, paneBox{ID: id, X: x, Y: r.y - 1, W: max(2, unitsOf(r.x+r.w, g.bw)-x), H: r.h})
	}
	if rects["comp"].h == 0 { // hidden companion: keep a box under the others for when the avatar comes on
		low := 0
		for _, b := range out {
			low = max(low, b.Y+b.H)
		}
		out = append(out, paneBox{ID: "comp", X: 0, Y: low, W: layoutUnits, H: defaultCompH})
	}
	return out
}

// geometryFrom draws boxes: each rect is its units times the viewport width.
func (m Model) geometryFrom(boxes []paneBox, def dashGeom) dashGeom {
	g := dashGeom{wide: def.wide, bw: def.bw, bh: def.bh}
	for _, b := range boxes {
		x0, x1 := b.X*g.bw/layoutUnits, (b.X+b.W)*g.bw/layoutUnits
		r := rect{x0, 1 + b.Y, x1 - x0, b.H}
		switch b.ID {
		case "agents":
			g.agents = r
		case "usage":
			g.usage = r
		case "tasks":
			g.tasks = r
		case "workflows":
			g.wf = r
		case "comp":
			if m.avatar.enabled() {
				g.comp = r
			}
		}
	}
	g.usageLines = m.usageLines(g.usage.w-4, g.usage.h-2)
	return g
}

// customBoxes is the layout in effect for this width: the one being edited, else
// the saved one merged onto the defaults. ok is false with no saved layout.
func (m Model) customBoxes(def dashGeom) ([]paneBox, bool) {
	if m.layoutEdit {
		return m.layoutDraft, true
	}
	saved := m.layouts.boxes(def.wide)
	if saved == nil {
		return nil, false
	}
	return mergeBoxes(saved, boxesFrom(def), minPaneH, ""), true
}

// wideLayout is the width class: wide keeps Tasks beside the left column.
func (m Model) wideLayout() bool {
	w := m.width
	if w <= 0 {
		w = fallbackWidth
	}
	return w >= 90
}
