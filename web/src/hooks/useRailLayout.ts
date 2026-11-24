import { useState } from "react"

export type RailId = "overview" | "agents" | "tasks"
const DEFAULT: RailId[] = ["overview", "agents", "tasks"]
const KEY = "tagent.rail"

type Saved = { order: RailId[]; heights: Partial<Record<RailId, number>> }

// pcts keeps only percentage heights (0–100]; older pixel heights are dropped.
const pcts = (h: unknown): Saved["heights"] =>
  h && typeof h === "object" ? Object.fromEntries(Object.entries(h).filter(([, v]) => typeof v === "number" && v > 0 && v <= 100)) : {}

const load = (): Saved => {
  try {
    const s = JSON.parse(localStorage.getItem(KEY) ?? "{}")
    const ok = Array.isArray(s.order) && s.order.length === 3 && DEFAULT.every((id) => s.order.includes(id))
    return { order: ok ? s.order : DEFAULT, heights: pcts(s.heights) }
  } catch {
    return { order: DEFAULT, heights: {} }
  }
}

// useRailLayout remembers (per device) the order of the rail panels and the
// panel heights the user dragged to, as a percentage of the rail; no height = automatic.
export function useRailLayout() {
  const [s, setS] = useState(load)
  const set = (n: Saved) => {
    setS(n)
    try { localStorage.setItem(KEY, JSON.stringify(n)) } catch { /* not remembered in private mode */ }
  }
  return {
    order: s.order,
    heights: s.heights,
    setHeight: (id: RailId, h: number | null) => {
      const heights = { ...s.heights }
      if (h === null) delete heights[id]; else heights[id] = h
      set({ ...s, heights })
    },
    // move swaps id with its neighbour among the visible panels.
    move: (id: RailId, d: -1 | 1, visible: RailId[]) => {
      const j = visible.indexOf(id) + d
      const other = visible[j]
      if (!other) return
      const o = [...s.order]
      const a = o.indexOf(id), b = o.indexOf(other)
      ;[o[a], o[b]] = [o[b]!, o[a]!]
      set({ ...s, order: o })
    },
  }
}
