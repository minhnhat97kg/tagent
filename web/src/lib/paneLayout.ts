// The home layout is split fractions: the three columns (Agents, Chat, Tasks) and the left column's
// two rows (Agents over Usage). Saved per device. Anything but a v:2 layout reads as none (the defaults).

export type PaneSizes = { cols: number[]; left: number[] }

export const DEFAULT_PANES: PaneSizes = { cols: [0.22, 0.43, 0.35], left: [0.65, 0.35] }

const KEY = "tagent.panes"

const isFractions = (v: unknown, n: number): v is number[] =>
  Array.isArray(v) && v.length === n && v.every(x => typeof x === "number" && Number.isFinite(x) && x >= 0 && x <= 1)
  && Math.abs(v.reduce((sum, x) => sum + x, 0) - 1) < 0.01

// loadPanes is this device's saved layout; undefined means none, an old grid (v:1) or one that cannot be read.
export function loadPanes(): PaneSizes | undefined {
  try {
    const s = JSON.parse(localStorage.getItem(KEY) ?? "null")
    return s?.v === 2 && isFractions(s.cols, 3) && isFractions(s.left, 2) ? { cols: s.cols, left: s.left } : undefined
  } catch {
    return undefined
  }
}

// savePanes stores the layout; undefined removes it.
export function savePanes(panes?: PaneSizes) {
  try {
    if (panes) localStorage.setItem(KEY, JSON.stringify({ v: 2, cols: panes.cols, left: panes.left }))
    else localStorage.removeItem(KEY)
  } catch { /* not remembered in private mode */ }
}
