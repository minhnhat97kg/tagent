// Pure layout for the workflow canvas: columns run left to right by longest path from the start,
// rows follow a barycenter pass, and edges are cubic curves that always go left to right.
export type Job = { ID: string; Status: string; Agent: string; Model: string; Prompt: string; Run: string; Uses?: string; Needs?: string[]; Outputs: string[]; OnFailure: string; MaxLoops: number }

export type NodeKind = 'start' | 'agent' | 'run' | 'status' | 'loop' | 'done'
export type GNode = { id: string; kind: NodeKind; title: string; chip?: string; note?: string; status?: string; target?: string }
export type GEdge = { from: string; to: string; label?: string }
export type Placed = GNode & { x: number; y: number }
export type PlacedEdge = GEdge & { d: string; lx: number; ly: number }
export type Graph = { nodes: Placed[]; edges: PlacedEdge[]; width: number; height: number }

export const NODE_W = 200, NODE_H = 60, GAP_X = 64, GAP_Y = 20, PAD = 24
export const START_ID = '__start', DONE_ID = '__done'

export function layoutGraph(input: GNode[], edges: GEdge[]): Graph {
  const list = [...new Map(input.map(n => [n.id, n])).values()]
  const ids = new Set(list.map(n => n.id))
  let valid = edges.filter(e => ids.has(e.from) && ids.has(e.to) && e.from !== e.to)
  // Longest path from the roots. A cycle never settles, so fall back to input order.
  const rank = new Map(list.map(n => [n.id, 0]))
  for (let pass = 0, changed = true; changed && pass <= list.length; pass++) {
    changed = false
    for (const e of valid) {
      const r = rank.get(e.from)! + 1
      if (r > rank.get(e.to)!) { rank.set(e.to, r); changed = true }
    }
    if (changed && pass === list.length) list.forEach((n, i) => rank.set(n.id, i))
  }
  valid = valid.filter(e => rank.get(e.from)! < rank.get(e.to)!)
  const preds = new Map(list.map(n => [n.id, [] as string[]]))
  valid.forEach(e => preds.get(e.to)!.push(e.from))

  const cols: string[][] = []
  for (const n of list) (cols[rank.get(n.id)!] ??= []).push(n.id)
  for (let c = 0; c < cols.length; c++) cols[c] ??= []

  // Barycenter: each column sorts by the mean row of its placed predecessors.
  const row = new Map<string, number>()
  cols.forEach((col, c) => {
    const key = (id: string, i: number) => {
      const ps = preds.get(id)!.filter(p => row.has(p))
      return ps.length ? ps.reduce((s, p) => s + row.get(p)!, 0) / ps.length : i
    }
    const order = col.map((id, i) => ({ id, k: key(id, i) })).sort((a, b) => a.k - b.k).map(x => x.id)
    order.forEach((id, i) => row.set(id, i))
    cols[c] = order
  })

  const tallest = Math.max(1, ...cols.map(c => c.length))
  const H = tallest * NODE_H + (tallest - 1) * GAP_Y
  const at = new Map<string, { x: number; y: number }>()
  cols.forEach((col, c) => {
    const h = col.length * NODE_H + Math.max(0, col.length - 1) * GAP_Y
    col.forEach((id, i) => at.set(id, { x: PAD + c * (NODE_W + GAP_X), y: PAD + (H - h) / 2 + i * (NODE_H + GAP_Y) }))
  })

  const placedEdges: PlacedEdge[] = valid.map(e => {
    const s = at.get(e.from)!, t = at.get(e.to)!
    const x1 = s.x + NODE_W, y1 = s.y + NODE_H / 2, x2 = t.x, y2 = t.y + NODE_H / 2
    const dx = Math.max(40, (x2 - x1) / 2)
    return { ...e, d: `M${x1},${y1} C${x1 + dx},${y1} ${x2 - dx},${y2} ${x2},${y2}`, lx: (x1 + x2) / 2, ly: (y1 + y2) / 2 }
  })
  const C = Math.max(1, cols.length)
  return {
    nodes: list.map(n => ({ ...n, ...at.get(n.id)! })),
    edges: placedEdges,
    width: PAD * 2 + C * NODE_W + (C - 1) * GAP_X,
    height: PAD * 2 + H,
  }
}

// jobsToGraph maps workflow jobs to nodes. needs are the edges; with no needs anywhere the
// runtime chains jobs in order, so the graph does too. on-failure becomes its own loop node.
export function jobsToGraph(jobs: Job[]): { nodes: GNode[]; edges: GEdge[] } {
  const uniq = [...new Map(jobs.map(j => [j.ID, j])).values()]
  const index = new Map(uniq.map((j, i) => [j.ID, i]))
  const declares = uniq.some(j => (j.Needs ?? []).length > 0)
  const nodes: GNode[] = [{ id: START_ID, kind: 'start', title: 'Start' }]
  const edges: GEdge[] = []
  const dependents = new Set<string>()
  if (!uniq.length) edges.push({ from: START_ID, to: DONE_ID })
  uniq.forEach((j, i) => {
    const kind: NodeKind = j.Run ? 'run' : j.Agent || j.Prompt || j.Model || j.Uses ? 'agent' : 'status'
    const chip = j.Run ? 'command' : j.Model ? (j.Agent ? `${j.Agent} · ${j.Model}` : j.Model) : j.Agent || (j.Uses ? `uses ${j.Uses}` : '')
    const target = j.OnFailure
    const loop = !!target && target !== j.ID && index.has(target)
    nodes.push({ id: j.ID, kind, title: j.ID, chip, status: j.Status, note: !loop && (j.OnFailure || j.Run) ? `fail → ${target || 'blocked'}` : undefined })
    const from = declares ? [...new Set((j.Needs ?? []).filter(n => index.has(n) && n !== j.ID))] : i > 0 ? [uniq[i - 1].ID] : []
    if (!from.length) edges.push({ from: START_ID, to: j.ID })
    for (const p of from) {
      dependents.add(p)
      edges.push({ from: p, to: j.ID, label: uniq[index.get(p)!].Run ? 'pass' : undefined })
    }
    if (loop) {
      const id = 'loop:' + j.ID
      nodes.push({ id, kind: 'loop', title: `${index.get(target)! <= i ? 'back to' : 'to'} ${target}`, target })
      edges.push({ from: j.ID, to: id, label: (j.Run ? 'fail' : 'changes') + (j.MaxLoops ? ` · max ${j.MaxLoops}` : '') })
    }
  })
  if (uniq.length) {
    nodes.push({ id: DONE_ID, kind: 'done', title: 'Done' })
    for (const j of uniq) if (!dependents.has(j.ID)) edges.push({ from: j.ID, to: DONE_ID })
  } else nodes.push({ id: DONE_ID, kind: 'done', title: 'Done' })
  return { nodes, edges }
}

// fitView returns the zoom and offset that show content inside a box, never zooming past 1.
export function fitView(content: { w: number; h: number }, box: { w: number; h: number }): { x: number; y: number; k: number } {
  const k = Math.min(1, box.w / content.w, box.h / content.h)
  const scale = Number.isFinite(k) && k > 0 ? k : 1
  return { k: scale, x: (box.w - content.w * scale) / 2, y: (box.h - content.h * scale) / 2 }
}
