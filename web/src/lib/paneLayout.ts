// The desktop home layout is a tree: a pane, or a split of nodes side by side (x) or stacked (y) with their
// fractions of it. Every pane appears exactly once, a split has at least two children, and none has the axis of
// its parent. Saved per device. The older v:2 and v:3 layouts are converted; anything else reads as none (the
// default). The operations here are pure.
//
// A pane is one of the four built-in ones or a custom pane, "ext:<name>". A layout holds any of them, at least
// one; a built-in that is not in it can be added back.

export type PaneId = string
export type Axis = "x" | "y"
export type Split = { axis: Axis; sizes: number[]; children: Layout[] }
export type Layout = PaneId | Split
// A side of a target node to land beside, or its place.
export type Side = "left" | "right" | "above" | "below"
export type Zone = Side | "swap"
export type Rect = { left: number; top: number; width: number; height: number }
// A path leads from the root to a node by child indexes; [] is the whole layout.
export type Path = number[]

export const PANES: PaneId[] = ["agents", "usage", "chat", "tasks"]
export const TITLES: Record<string, string> = { agents: "Agents", usage: "Usage", chat: "Chat", tasks: "Tasks" }
export const extId = (name: string) => `ext:${name}`
// extName is the custom pane's name, or undefined for a built-in (or anything else).
export const extName = (id: PaneId) => (/^ext:[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(id) ? id.slice(4) : undefined)
export const paneTitle = (id: PaneId) => TITLES[id] ?? extName(id) ?? id
export const DEFAULT_LAYOUT: Split = {
  axis: "x", sizes: [0.22, 0.43, 0.35],
  children: [{ axis: "y", sizes: [0.65, 0.35], children: ["agents", "usage"] }, "chat", "tasks"],
}
const MIN_WIDTH: Record<string, number> = { agents: 220, usage: 220, chat: 220, tasks: 280 }
const minWidth = (id: PaneId) => MIN_WIDTH[id] ?? 220
export const MIN_HEIGHT = 128
export const SHUT_HEIGHT = 32 // the reopen button of a shut Usage
const EDGE = 0.25 // this share of a pane's width or height, from each side, docks beside it

const sum = (a: number[]) => a.reduce((s, v) => s + v, 0)
const fractions = (a: number[]) => { const s = sum(a); return a.map(v => (s > 0 ? v / s : 1 / a.length)) }

// The sizes a double-click on a divider restores.
export const defaultSizes = (axis: Axis, n: number) =>
  axis === "x" && n === 3 ? DEFAULT_LAYOUT.sizes : axis === "y" && n === 2 ? [0.65, 0.35] : Array<number>(n).fill(1 / n)

export const panesOf = (node: Layout): PaneId[] => (typeof node === "string" ? [node] : node.children.flatMap(panesOf))
// shape names the arrangement without the sizes.
export const shape = (node: Layout): string => (typeof node === "string" ? node : `${node.axis}(${node.children.map(shape).join(" ")})`)
export const nodeAt = (root: Layout, path: Path): Layout => path.reduce((node, i) => (node as Split).children[i]!, root)
export function pathOf(node: Layout, id: PaneId): Path | undefined {
  if (typeof node === "string") return node === id ? [] : undefined
  for (const [i, child] of node.children.entries()) {
    const rest = pathOf(child, id)
    if (rest) return [i, ...rest]
  }
  return undefined
}
// replace puts node at path.
export function replace(root: Layout, path: Path, node: Layout): Layout {
  const [i, ...rest] = path
  if (i === undefined) return node
  const split = root as Split
  return { ...split, children: split.children.map((c, k) => (k === i ? replace(c, rest, node) : c)) }
}

// removePane takes a pane out; its neighbours share its room. The last pane stays: undefined.
export function removePane(root: Layout, id: PaneId): Layout | undefined {
  const rest = panesOf(root).includes(id) ? without(root, id) : undefined
  return rest && normalize(rest)
}

// prune drops the panes keep refuses, e.g. custom panes that are gone. All gone: undefined (the default).
export function prune(root: Layout, keep: (id: PaneId) => boolean): Layout | undefined {
  return panesOf(root).filter(id => !keep(id)).reduce<Layout | undefined>((l, id) => l && removePane(l, id), root)
}

// addPane puts a pane that is not in the layout beside the rest, in a new column (a fifth to a half of the width), or in a new row when the layout already has four columns.
export function addPane(root: Layout, id: PaneId): Layout {
  if (panesOf(root).includes(id)) return root
  const rows = typeof root !== "string" && root.axis === "x" && root.children.length >= 4
  const share = rows ? 0.3 : Math.max(0.2, 1 / (panesOf(root).length + 1))
  return normalize({ axis: rows ? "y" : "x", sizes: [1 - share, share], children: [root, id] })
}

// shutIndex is where a split holds a shut Usage, or -1. Only Usage shuts, and only when it is stacked.
export const shutIndex = (s: Split) => (s.axis === "y" ? s.children.findIndex((c, i) => c === "usage" && s.sizes[i] === 0) : -1)
// buttonHolder is the child that carries the reopen button: the one above the shut pane, else the one below it.
export const buttonHolder = (shut: number) => (shut < 0 ? -1 : shut > 0 ? shut - 1 : 1)

// normalize merges a split into a parent of the same axis, replaces a split of one by that child, and makes the
// fractions add up to 1.
export function normalize(node: Layout): Layout {
  if (typeof node === "string") return node
  const parts = node.children.flatMap((child, i) => {
    const n = normalize(child)
    const size = node.sizes[i]!
    return typeof n !== "string" && n.axis === node.axis ? n.children.map((c, k) => [c, size * n.sizes[k]!] as const) : [[n, size] as const]
  })
  if (parts.length === 1) return parts[0]![0]
  // Side by side nothing is shut: a Usage that arrives there without a size gets an even share.
  const sizes = parts.map(([c, size]) => (size === 0 && !(node.axis === "y" && c === "usage") ? 1 / parts.length : size))
  return { axis: node.axis, sizes: fractions(sizes), children: parts.map(p => p[0]) }
}

// reopen gives a shut Usage its default share of its stack again.
export function reopen(node: Layout): Layout {
  if (typeof node === "string") return node
  const shut = shutIndex(node)
  const share = defaultSizes("y", 2)[1]!
  const sizes = shut < 0 ? node.sizes : node.sizes.map((s, i) => (i === shut ? share : s * (1 - share)))
  return { ...node, sizes, children: node.children.map(reopen) }
}

const NEW: PaneId = "new" // holds the moved pane's place until its old one is gone
const without = (node: Layout, id: PaneId): Layout | undefined => {
  if (typeof node === "string") return node === id ? undefined : node
  const kept = node.children.flatMap((c, i) => { const n = without(c, id); return n ? [[n, node.sizes[i]!] as const] : [] })
  return { axis: node.axis, sizes: fractions(kept.map(k => k[1])), children: kept.map(k => k[0]) }
}
const rename = (node: Layout, names: Partial<Record<string, PaneId>>): Layout =>
  typeof node === "string" ? names[node] ?? node : { ...node, children: node.children.map(c => rename(c, names)) }
const axisOf = (side: Side): Axis => (side === "left" || side === "right" ? "x" : "y")
const leads = (side: Side) => side === "left" || side === "above"

// dock moves pane id to a side of the node at path, which it then spans, or swaps it with the pane there. The
// pane's old place goes to its neighbours; a new neighbour takes half of the node's size. undefined means
// nothing would change.
export function dock(root: Layout, id: PaneId, path: Path, zone: Zone): Layout | undefined {
  const target = nodeAt(root, path)
  if (target === id) return undefined
  const parent = path.length ? (nodeAt(root, path.slice(0, -1)) as Split) : undefined
  const at = path.at(-1)!
  const shut = !!parent && parent.sizes[at] === 0
  let next: Layout
  if (zone === "swap") {
    if (typeof target !== "string" || shut) return undefined
    next = rename(root, { [id]: target, [target]: id })
  } else {
    const first = leads(zone)
    let grown: Layout
    if (shut) { // nothing to split: join the stack with an even share
      const share = 1 / (parent.children.length + 1)
      const sizes = parent.sizes.map(s => s * (1 - share))
      const children = parent.children.slice()
      const i = first ? at : at + 1
      sizes.splice(i, 0, share)
      children.splice(i, 0, NEW)
      grown = replace(root, path.slice(0, -1), { axis: parent.axis, sizes, children })
    } else {
      grown = replace(root, path, { axis: axisOf(zone), sizes: [0.5, 0.5], children: first ? [NEW, target] : [target, NEW] })
    }
    next = normalize(rename(without(grown, id)!, { [NEW]: id }))
  }
  return shape(next) === shape(root) ? undefined : next
}

const ARROWS: Partial<Record<string, Side>> = { ArrowLeft: "left", ArrowRight: "right", ArrowUp: "above", ArrowDown: "below" }

// step is an arrow key on a pane. Up and down move it within its stack; left and right into the neighbouring
// column at the same row. With nowhere to go it moves out, to span the next larger group on that side.
export function step(root: Layout, id: PaneId, key: string): Layout | undefined {
  const side = ARROWS[key]
  if (!side) return undefined
  const path = pathOf(root, id)!
  const d = leads(side) ? -1 : 1
  const parent = nodeAt(root, path.slice(0, -1)) as Split
  const row = path.at(-1)!
  if (axisOf(side) === "y") {
    if (parent.axis === "y" && parent.children[row + d] !== undefined) {
      const swap = <T,>(a: T[]) => { const out = a.slice(); out[row] = a[row + d]!; out[row + d] = a[row]!; return out }
      return replace(root, path.slice(0, -1), { axis: "y", sizes: swap(parent.sizes), children: swap(parent.children) })
    }
  } else {
    // A pane in a row looks only at that row; a stacked one at the nearest row with a neighbour on that side.
    for (let k = path.length - 1; k >= (parent.axis === "x" ? path.length - 1 : 0); k--) {
      const group = nodeAt(root, path.slice(0, k)) as Split
      const beside = group.children[path[k]! + d]
      if (group.axis !== "x" || beside === undefined) continue
      const to = [...path.slice(0, k), path[k]! + d]
      const j = parent.axis === "y" ? row : 0
      if (typeof beside === "string") return dock(root, id, to, j === 0 ? "above" : "below")
      const n = beside.children.length
      return dock(root, id, [...to, Math.min(j, n - 1)], j < n ? "above" : "below")
    }
  }
  for (let k = path.length - 1; k >= 0; k--) {
    const next = dock(root, id, path.slice(0, k), side)
    if (next) return next
  }
  return undefined
}

// spans lists the groups, outermost first, that share a side of the node at path and that docking there would
// span: those split the other way.
function spans(root: Layout, path: Path, side: Side): Path[] {
  const out: Path[] = []
  for (let k = path.length - 1; k >= 0; k--) {
    const group = nodeAt(root, path.slice(0, k)) as Split
    if (group.axis !== axisOf(side)) { out.unshift(path.slice(0, k)); continue }
    const open = group.children.flatMap((_, i) => (i === shutIndex(group) ? [] : [i]))
    if (path[k] !== (leads(side) ? open[0] : open.at(-1))) break // this edge is inside the group
  }
  return out
}

export type Drop = { next: Layout; zone: Zone; path: Path }

// dropAt is what releasing pane id at (x, y) in the width by height pane target would do. From a side of the
// pane inwards: a band px wide strip for each larger group sharing that side, the widest group first, then the
// pane's own edge (a quarter of it); the middle swaps. Only places that change the layout and pass ok are
// offered, and only those take room. undefined: nothing.
export function dropAt(root: Layout, id: PaneId, target: PaneId, x: number, y: number, width: number, height: number, band: number, ok: (next: Layout) => boolean): Drop | undefined {
  const path = pathOf(root, target)!
  const offer = (to: Path, zone: Zone): Drop | undefined => {
    const next = dock(root, id, to, zone)
    return next && ok(next) ? { next, zone, path: to } : undefined
  }
  const hits = (["left", "right", "above", "below"] as Side[]).flatMap(side => {
    const size = axisOf(side) === "x" ? width : height
    const d = side === "left" ? x : side === "right" ? width - x : side === "above" ? y : height - y
    const own = offer(path, side)
    const seen = new Set(own ? [shape(own.next)] : [])
    const groups = spans(root, path, side).flatMap(to => {
      const g = offer(to, side)
      if (!g || seen.has(shape(g.next))) return []
      seen.add(shape(g.next))
      return [g]
    })
    const strip = Math.min(band, (0.4 * size) / (groups.length + 1)) // small panes keep a middle
    if (d < groups.length * strip) return [{ rank: 0, near: d, drop: groups[Math.max(0, Math.floor(d / strip))] }]
    const reach = Math.max(EDGE * size, groups.length ? (groups.length + 1) * strip : 0)
    return d < reach ? [{ rank: 1, near: d / size, drop: own }] : []
  }).sort((a, b) => a.rank - b.rank || a.near - b.near)
  return hits.length ? hits.find(h => h.drop)?.drop : offer(path, "swap")
}

// minSize is the smallest width and height that hold every pane of a node at its minimum, with gap px per divider.
export function minSize(node: Layout, gap: number): { width: number; height: number } {
  if (typeof node === "string") return { width: minWidth(node), height: MIN_HEIGHT }
  const shut = shutIndex(node)
  const open = node.children.filter((_, i) => i !== shut).map(c => minSize(c, gap))
  const gaps = gap * (open.length - 1)
  return node.axis === "x"
    ? { width: sum(open.map(o => o.width)) + gaps, height: Math.max(...open.map(o => o.height)) }
    : { width: Math.max(...open.map(o => o.width)), height: sum(open.map(o => o.height)) + gaps + (shut < 0 ? 0 : SHUT_HEIGHT) }
}

// fits is whether next may replace current in a window: it holds every minimum, or at least needs no more than
// current does (a window already too small can still be tidied). A size of 0 is not measured: anything fits.
export function fits(next: Layout, current: Layout, width: number, height: number, gap: number): boolean {
  const n = minSize(next, gap)
  const c = minSize(current, gap)
  return (!width || n.width <= Math.max(width, c.width)) && (!height || n.height <= Math.max(height, c.height))
}

// distribute shares avail px by the fractions, as the flex row does: a child under its minimum is held there.
function distribute(parts: number[], mins: number[], avail: number): number[] {
  const held = new Set<number>()
  for (;;) {
    const free = avail - sum([...held].map(i => mins[i]!))
    const total = sum(parts.filter((_, i) => !held.has(i)))
    const out = parts.map((f, i) => (held.has(i) ? mins[i]! : total > 0 ? (free * f) / total : 0))
    const under = out.flatMap((v, i) => (!held.has(i) && v < mins[i]! ? [i] : []))
    if (!under.length) return out
    under.forEach(i => held.add(i))
  }
}

// cells is a split in rect: each child's minimum along the axis, its px along it (the button's holder includes
// the button) and its own area. A shut child has none. Minimums that do not fit shrink by the same factor.
export function cells(split: Split, rect: Rect, gap: number): { min: number; px: number; rect: Rect }[] {
  const x = split.axis === "x"
  const shut = shutIndex(split)
  const holder = buttonHolder(shut)
  const open = split.children.flatMap((_, i) => (i === shut ? [] : [i]))
  const avail = (x ? rect.width : rect.height) - gap * (open.length - 1)
  const wanted = open.map(i => { const m = minSize(split.children[i]!, gap); return x ? m.width : m.height + (i === holder ? SHUT_HEIGHT : 0) })
  const need = sum(wanted)
  const mins = avail > 0 && need > avail ? wanted.map(m => (m * avail) / need) : wanted
  const px = distribute(open.map(i => split.sizes[i]!), mins, avail)
  let offset = 0
  return split.children.map((_, i) => {
    const k = open.indexOf(i)
    if (k < 0) return { min: 0, px: 0, rect: { ...rect, height: 0 } }
    const size = px[k]!
    const button = i === holder ? SHUT_HEIGHT : 0
    const area = x
      ? { left: rect.left + offset, top: rect.top, width: size, height: rect.height }
      : { left: rect.left, top: rect.top + offset + (shut < i ? button : 0), width: rect.width, height: size - button }
    offset += size + gap
    return { min: mins[k]!, px: size, rect: area }
  })
}

// settle rewrites the fractions to what the window will show, so a divider starts from the size on screen.
export function settle(root: Layout, width: number, height: number, gap: number): Layout {
  if (width <= 0 || height <= 0) return root
  const fit = (node: Layout, rect: Rect): Layout => {
    if (typeof node === "string") return node
    const list = cells(node, rect, gap)
    return { axis: node.axis, sizes: fractions(list.map(c => c.px)), children: node.children.map((c, i) => fit(c, list[i]!.rect)) }
  }
  return fit(root, { left: 0, top: 0, width, height })
}

// paneRect is the area pane id takes in the window.
export function paneRect(root: Layout, id: PaneId, width: number, height: number, gap: number): Rect {
  let node = root
  let rect: Rect = { left: 0, top: 0, width, height }
  for (const i of pathOf(root, id)!) {
    rect = cells(node as Split, rect, gap)[i]!.rect
    node = (node as Split).children[i]!
  }
  return rect
}

const KEY = "tagent.panes"

const isFractions = (v: unknown, n: number): v is number[] =>
  Array.isArray(v) && v.length === n && v.every(x => typeof x === "number" && Number.isFinite(x) && x >= 0 && x <= 1)
  && Math.abs(sum(v) - 1) < 0.01

const isPanes = (v: unknown): v is PaneId[] => Array.isArray(v) && v.length === PANES.length && PANES.every(id => v.includes(id))

// A saved node is usable when only a stacked Usage has no size.
function isNode(v: unknown): v is Layout {
  if (typeof v === "string") return PANES.includes(v) || extName(v) !== undefined
  const s = v as Split | null
  return (s?.axis === "x" || s?.axis === "y") && Array.isArray(s.children) && s.children.length > 0 && isFractions(s.sizes, s.children.length)
    && s.children.every((c, i) => isNode(c) && (s.sizes[i]! > 0 || (s.axis === "y" && c === "usage")))
}
// Panes appear once each. A v:4 layout may lack built-in panes or hold custom ones; the older kinds hold all four.
const once = (ids: PaneId[]) => new Set(ids).size === ids.length
const usable = (v: unknown, all = true): Layout | undefined => (isNode(v) && (all ? isPanes(panesOf(v)) : once(panesOf(v))) ? normalize(v) : undefined)

// v:3 was a row of columns, each a stack of panes.
function fromColumns(columns: unknown): Layout | undefined {
  if (!Array.isArray(columns)) return undefined
  return usable({
    axis: "x", sizes: columns.map(c => c?.width),
    children: columns.map(c => ({ axis: "y", sizes: c?.heights, children: c?.panes })),
  })
}

// v:2 had three columns, the first with two rows, and an optional order of the panes in those four places.
function fromV2(s: { cols?: unknown; left?: unknown; order?: unknown }): Layout | undefined {
  if (!isFractions(s.cols, 3) || !isFractions(s.left, 2)) return undefined
  const [a, b, c, d] = (isPanes(s.order) ? s.order : PANES) as [PaneId, PaneId, PaneId, PaneId]
  // Any pane could be shut in the lower left place; now only Usage can.
  const open = [a, b].every((id, i) => (s.left as number[])[i]! > 0 || id === "usage")
  return fromColumns([
    { width: s.cols[0]!, panes: [a, b], heights: open ? s.left : defaultSizes("y", 2) },
    { width: s.cols[1]!, panes: [c], heights: [1] },
    { width: s.cols[2]!, panes: [d], heights: [1] },
  ])
}

// loadPanes is this device's saved layout; undefined means none or one that cannot be read.
export function loadPanes(): Layout | undefined {
  try {
    const s = JSON.parse(localStorage.getItem(KEY) ?? "null")
    return s?.v === 4 ? usable(s.root, false) : s?.v === 3 ? fromColumns(s.columns) : s?.v === 2 ? fromV2(s) : undefined
  } catch {
    return undefined
  }
}

// savePanes stores the layout; undefined removes it.
export function savePanes(layout?: Layout) {
  try {
    if (layout) localStorage.setItem(KEY, JSON.stringify({ v: 4, root: layout }))
    else localStorage.removeItem(KEY)
  } catch { /* not remembered in private mode */ }
}
