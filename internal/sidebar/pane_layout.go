package sidebar

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"
)

// The layout mode (M on the dashboard) owns every key except quit: h j k l move
// the selected pane, H J K L resize it, tab selects, enter saves, esc discards,
// 0 resets this width's layout to the default.

// startLayoutEdit enters the mode with the saved (or default) boxes; the
// selection starts at the focused card.
func (m Model) startLayoutEdit() Model {
	m.layoutEdit, m.layoutDraft, m.layoutSel = true, m.editBoxes(), cardIDs[m.focus]
	m.drag = nil
	return m
}

// editBoxes is the boxes a layout edit starts from: the custom ones, else the default.
func (m Model) editBoxes() []paneBox {
	def := m.defaultGeometry()
	boxes, ok := m.customBoxes(def)
	if !ok {
		boxes = mergeBoxes(boxesFrom(def), boxesFrom(def), minPaneH, "") // a short default can give a card fewer rows than its minimum
	}
	return boxes
}

func layoutHints(Model) []hint {
	return []hint{
		{"h/j/k/l", "move", "h j k l", "move the selected pane (arrows too)"},
		{"H/J/K/L", "resize", "H J K L", "shrink width, grow height, shrink height, grow width"},
		{"tab/S-Tab", "select", "Tab S-Tab", "select the next / previous pane"},
		{"enter", "save", "enter", "save the layout and leave"},
		{"esc", "discard", "esc", "discard the changes and leave"},
		{"0", "reset", "0", "reset this width's layout to the default and leave"},
	}
}

func (m Model) layoutKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "q", "ctrl+c": // quit is not a layout key
		return m, nil, false
	case "esc":
		m.layoutEdit, m.layoutDraft, m.drag = false, nil, nil
	case "enter", "0":
		boxes := m.layoutDraft
		if key == "0" {
			boxes = nil
		}
		save := key == "0" || m.layoutDirty
		m.layoutEdit, m.layoutDraft, m.layoutDirty, m.drag = false, nil, false, nil
		if save {
			m.layouts.set(m.wideLayout(), boxes)
			if err := saveLayout(m.wideLayout(), boxes); err != nil {
				m.taskNotice = "! Layout: " + err.Error()
			}
		}
	case "tab", "shift+tab":
		shown := m.layoutShown()
		step := 1
		if key == "shift+tab" {
			step = len(shown) - 1
		}
		m.layoutSel = shown[(slices.Index(shown, m.layoutSel)+step)%len(shown)]
		m = m.followPane(m.layoutSel)
	case "h", "left":
		m = m.nudgeSelected(-1, 0, 0, 0)
	case "l", "right":
		m = m.nudgeSelected(1, 0, 0, 0)
	case "k", "up":
		m = m.nudgeSelected(0, -1, 0, 0)
	case "j", "down":
		m = m.nudgeSelected(0, 1, 0, 0)
	case "H":
		m = m.nudgeSelected(0, 0, -1, 0)
	case "L":
		m = m.nudgeSelected(0, 0, 1, 0)
	case "J":
		m = m.nudgeSelected(0, 0, 0, 1)
	case "K":
		m = m.nudgeSelected(0, 0, 0, -1)
	}
	return m, nil, true
}

// layoutShown is the selectable panes in Tab order: the companion only while the avatar is on.
func (m Model) layoutShown() []string {
	var out []string
	for _, id := range layoutIDs {
		if id != "comp" || m.avatar.enabled() {
			out = append(out, id)
		}
	}
	return out
}

// nudgeSelected is placeSelected, then the selected pane comes into view.
func (m Model) nudgeSelected(dx, dy, dw, dh int) Model {
	m = m.placeSelected(dx, dy, dw, dh)
	return m.followPane(m.layoutSel)
}

// paneDrag is a mouse drag on the dashboard: the pane under the press (its top
// row moves it, its bottom-right corner resizes it), the canvas cell of the press
// and the boxes at the press. Every motion recomputes from base.
type paneDrag struct {
	id     string
	resize bool
	x, y   int // canvas cell of the press
	base   []paneBox
}

// startDrag grabs the pane under a press at canvas cell (x, y), if any.
func (m Model) startDrag(g dashGeom, x, y int) Model {
	rects := g.rects()
	for _, id := range layoutIDs {
		r := rects[id]
		if r.h == 0 || x < r.x || x >= r.x+r.w || y < r.y || y >= r.y+r.h {
			continue
		}
		resize := y == r.y+r.h-1 && x >= r.x+r.w-2
		if y != r.y && !resize {
			continue
		}
		m.scrollX, m.scrollY = g.clampScroll(m.scrollX, m.scrollY) // the canvas offset the press was translated with
		m.drag = &paneDrag{id: id, resize: resize, x: x, y: y, base: m.editBoxes()}
		return m
	}
	return m
}

// dragOK: a drag only moves panes on the plain dashboard.
func (m Model) dragOK() bool {
	return m.dashboard && !m.maximized && !m.subView() && m.taskForm == "" && !m.helpOpen && !m.taskDelete
}

// dragTo moves or resizes the dragged pane by the pointer's travel since the
// press (screen cell (x, y)). A drag that changes no box leaves no edit behind.
func (m Model) dragTo(x, y int) Model {
	d := m.drag
	dx, dy := x+m.scrollX-d.x, y+m.scrollY-d.y
	ux := unitsOf(max(dx, -dx), m.defaultGeometry().bw) // unitsOf rounds negatives wrongly
	if dx < 0 {
		ux = -ux
	}
	m.layoutSel, m.layoutDraft = d.id, d.base
	if d.resize {
		m = m.placeSelected(0, 0, ux, dy)
	} else {
		m = m.placeSelected(ux, dy, 0, 0)
	}
	changed := !slices.Equal(m.layoutDraft, d.base)
	m.layoutEdit, m.layoutDirty = changed, changed
	if !changed {
		m.layoutDraft = nil
	}
	return m
}

// endDrag is the release: a changed layout is saved as enter saves it.
func (m Model) endDrag() Model {
	if m.layoutEdit {
		m, _, _ = m.layoutKey("enter")
	}
	m.drag = nil
	return m
}

// cancelDrag is a press while a drag is open (its release was lost): the drag is discarded.
func (m Model) cancelDrag() Model {
	if m.layoutEdit {
		m, _, _ = m.layoutKey("esc")
	}
	m.drag = nil
	return m
}

// placeSelected moves and resizes the selected box in units and rows, keeps it
// within its limits, then moves the others out of its way.
func (m Model) placeSelected(dx, dy, dw, dh int) Model {
	boxes := slices.Clone(m.layoutDraft)
	for i := range boxes {
		if boxes[i].ID == m.layoutSel {
			b := boxes[i]
			b.X, b.Y, b.W, b.H = b.X+dx, b.Y+dy, b.W+dw, b.H+dh
			boxes[i] = normalizeBox(b, minPaneH)
		}
	}
	// Unknown ids (kept in storage, never drawn) sit after the built-in boxes and must not push them.
	var known, unknown []paneBox
	for _, b := range boxes {
		if slices.Contains(layoutIDs, b.ID) {
			known = append(known, b)
		} else {
			unknown = append(unknown, b)
		}
	}
	m.layoutDraft = append(placeBoxes(known, m.layoutSel), unknown...)
	m.layoutDirty = true
	return m
}

// followPane scrolls the canvas until the card is in view. The far edge comes
// first, so the near edge wins when the card is bigger than the viewport.
func (m Model) followPane(id string) Model {
	g := m.dashGeometry()
	r := g.rects()[id]
	if r.h == 0 {
		return m
	}
	x, y := g.clampScroll(m.scrollX, m.scrollY) // a stored offset can be past the canvas after a resize
	if r.x+r.w > x+g.bw {
		x = r.x + r.w - g.bw
	}
	if r.x < x {
		x = r.x
	}
	if r.y+r.h > y+1+g.bh { // screen rows: the body starts at row 1
		y = r.y + r.h - 1 - g.bh
	}
	if r.y < y+1 {
		y = r.y - 1
	}
	m.scrollX, m.scrollY = g.clampScroll(x, y)
	return m
}

// scrollCanvas scrolls the canvas down (dy > 0) or up by dy rows.
func (m Model) scrollCanvas(g dashGeom, dy int) Model {
	x, y := g.clampScroll(m.scrollX, m.scrollY)
	m.scrollX, m.scrollY = g.clampScroll(x, y+dy)
	return m
}
