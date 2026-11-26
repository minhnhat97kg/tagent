// The home pane grid. A port of internal/sidebar/panegrid.go, pinned by testdata/pane-layout.json.
// x and w are twelfths of the grid width (w and x may run past 12, up to 24); y and h are 24px rows.

export type PaneBox = { id: string; x: number; y: number; w: number; h: number }
export type PaneId = "usage" | "agents" | "tasks" | "workflows"

const MAX_UNIT = 24
const MAX_ROW = 500
const KEY = "tagent.panes"

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v))

export function normalizeBox(b: PaneBox, minH: number): PaneBox {
  const w = clamp(b.w, 2, MAX_UNIT)
  return { ...b, w, x: clamp(b.x, 0, MAX_UNIT - w), y: clamp(b.y, 0, MAX_ROW), h: clamp(b.h, minH, MAX_ROW) }
}

const overlaps = (a: PaneBox, b: PaneBox) => a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h

// placeBoxes keeps the moved box and moves every other box down, in (y, x) order, to the first row
// where it overlaps nothing placed. The input is not changed; the output keeps the input order.
export function placeBoxes(boxes: PaneBox[], moved: string): PaneBox[] {
  const out = boxes.map(b => ({ ...b }))
  const placed = out.filter(b => b.id === moved)
  const rest = out.map((_, i) => i).filter(i => out[i]!.id !== moved).sort((a, c) => out[a]!.y - out[c]!.y || out[a]!.x - out[c]!.x)
  for (const i of rest) {
    const b = out[i]!
    while (placed.some(p => overlaps(b, p))) b.y++
    placed.push(b)
  }
  return out
}

// freeRow is the first row, above low, where b overlaps none of placed; low when there is none.
const freeRow = (placed: PaneBox[], b: PaneBox, low: number) => {
  for (let y = 0; y < low; y++) if (!placed.some(p => overlaps({ ...b, y }, p))) return y
  return low
}

// mergeBoxes is a saved layout over the defaults: saved boxes keep their place (clamped), a repeated id
// keeps its first box, missing defaults take the first free row above the lowest box, then placeBoxes runs with moved.
// Ids that are not defaults are appended unchanged and never push a drawn pane down.
export function mergeBoxes(saved: PaneBox[], defaults: PaneBox[], minH: number, moved: string): PaneBox[] {
  const known = new Set(defaults.map(d => d.id))
  const out: PaneBox[] = []
  const unknown: PaneBox[] = []
  const seen = new Set<string>()
  const add = (b: PaneBox) => {
    if (seen.has(b.id)) return
    seen.add(b.id)
    if (known.has(b.id)) out.push(normalizeBox(b, minH))
    else unknown.push(b)
  }
  saved.forEach(add)
  const low = out.reduce((m, b) => Math.max(m, b.y + b.h), 0)
  for (const d of defaults) if (!seen.has(d.id)) add({ ...d, y: freeRow(out, d, low) })
  return [...placeBoxes(out, moved), ...unknown]
}

// defaultPanes fills a grid of rows rows: usage, agents and workflows stacked on the left, tasks beside them.
export function defaultPanes(rows: number): PaneBox[] {
  return [
    { id: "usage", x: 0, y: 0, w: 3, h: 10 },
    { id: "agents", x: 0, y: 10, w: 3, h: rows - 18 },
    { id: "workflows", x: 0, y: rows - 8, w: 3, h: 8 },
    { id: "tasks", x: 3, y: 0, w: 9, h: rows },
  ]
}

const isBox = (b: unknown): b is PaneBox => {
  const o = b as Partial<PaneBox> | null
  return !!o && typeof o.id === "string" && [o.x, o.y, o.w, o.h].every(n => Number.isInteger(n))
}

// loadPanes is this device's saved layout; undefined means none (or one that cannot be read).
export function loadPanes(): PaneBox[] | undefined {
  try {
    const s = JSON.parse(localStorage.getItem(KEY) ?? "null")
    return s?.v === 1 && Array.isArray(s.boxes) && s.boxes.every(isBox) ? s.boxes : undefined
  } catch {
    return undefined
  }
}

// savePanes stores the layout; undefined removes it.
export function savePanes(boxes?: PaneBox[]) {
  try {
    if (boxes) localStorage.setItem(KEY, JSON.stringify({ v: 1, boxes }))
    else localStorage.removeItem(KEY)
  } catch { /* not remembered in private mode */ }
}
