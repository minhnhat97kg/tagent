import { useEffect, useId, useLayoutEffect, useRef, useState, type PointerEvent } from 'react'
import { Bot, CheckCircle2, CircleDot, Maximize2, Terminal, Undo2, ZoomIn, ZoomOut } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { statusColor } from '@/components/BoardDrawer'
import { NODE_H, NODE_W, fitView, type Graph, type Placed } from '@/lib/graphLayout'

// FlowCanvas draws a laid-out graph as cards and curved edges. It knows nothing about workflows,
// so the routing page can feed it any Graph from layoutGraph.
const ICON = { start: CircleDot, done: CheckCircle2, agent: Bot, run: Terminal, status: CircleDot, loop: Undo2 }
const TINT = { start: 'text-muted-foreground', done: 'text-emerald-600', agent: 'text-indigo-600', run: 'text-sky-600', status: 'text-muted-foreground', loop: 'text-amber-600' }
const MIN_K = 0.4, MAX_K = 2

export function FlowCanvas({ graph, active, onSelect }: { graph: Graph; active?: string; onSelect: (id: string) => void }) {
  const box = useRef<HTMLDivElement>(null)
  const [size, setSize] = useState({ w: 0, h: 0 })
  const [view, setView] = useState({ x: 0, y: 0, k: 1 })
  // Pan and zoom survive polls; a new topology refits, and so does a resize until the user touches the view.
  const touched = useRef(false)
  const topo = useRef('')
  const drag = useRef<{ x: number; y: number; vx: number; vy: number } | null>(null)
  const moved = useRef(false)
  const marker = useId().replace(/:/g, '')
  const key = graph.nodes.map(n => n.id).join('|') + '#' + graph.edges.map(e => e.from + '>' + e.to).join('|')

  useLayoutEffect(() => {
    const el = box.current
    if (!el) return
    const measure = () => setSize({ w: el.clientWidth, h: el.clientHeight })
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  useEffect(() => {
    if (topo.current !== key) { topo.current = key; touched.current = false }
    if (!touched.current && size.w && size.h) setView(fitView({ w: graph.width, h: graph.height }, size))
  }, [key, size, graph.width, graph.height])
  useEffect(() => {
    const el = box.current
    if (!el) return
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      const r = el.getBoundingClientRect()
      zoomAt(Math.exp(-e.deltaY * 0.0015), e.clientX - r.left, e.clientY - r.top)
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [])

  function change(f: (v: { x: number; y: number; k: number }) => { x: number; y: number; k: number }) {
    touched.current = true
    setView(f)
  }
  function zoomAt(factor: number, cx: number, cy: number) {
    change(v => {
      const k = Math.min(MAX_K, Math.max(MIN_K, v.k * factor)), r = k / v.k
      return { k, x: cx - (cx - v.x) * r, y: cy - (cy - v.y) * r }
    })
  }
  const zoom = (factor: number) => zoomAt(factor, size.w / 2, size.h / 2)
  const fit = () => { touched.current = false; setView(fitView({ w: graph.width, h: graph.height }, size)) }

  function onPointerDown(e: PointerEvent<HTMLDivElement>) {
    if (e.button !== 0) return
    drag.current = { x: e.clientX, y: e.clientY, vx: view.x, vy: view.y }
    moved.current = false
  }
  function onPointerMove(e: PointerEvent<HTMLDivElement>) {
    const d = drag.current
    if (!d) return
    const dx = e.clientX - d.x, dy = e.clientY - d.y
    if (!moved.current && Math.hypot(dx, dy) < 4) return
    if (!moved.current) e.currentTarget.setPointerCapture(e.pointerId)
    moved.current = true
    change(v => ({ ...v, x: d.vx + dx, y: d.vy + dy }))
  }
  const endDrag = () => { drag.current = null }

  return <div className="flex h-full min-h-56 min-w-0 flex-col">
    <div ref={box} role="region" aria-label="Flow canvas" className="relative min-h-0 min-w-0 flex-1 touch-none select-none overflow-hidden rounded-lg border bg-background"
      style={{ backgroundImage: 'radial-gradient(color-mix(in srgb, var(--muted-foreground) 40%, transparent) 1px, transparent 1px)', backgroundSize: `${16 * view.k}px ${16 * view.k}px`, backgroundPosition: `${view.x}px ${view.y}px` }}
      onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={endDrag} onPointerCancel={endDrag}>
      <div className="absolute left-0 top-0 origin-top-left" style={{ width: graph.width, height: graph.height, transform: `translate(${view.x}px, ${view.y}px) scale(${view.k})` }}>
        <svg className="absolute left-0 top-0 overflow-visible text-muted-foreground" width={graph.width} height={graph.height} aria-hidden>
          <defs><marker id={marker} viewBox="0 0 8 8" refX="7" refY="4" markerWidth="6" markerHeight="6" orient="auto"><path d="M0,0 L8,4 L0,8 z" fill="currentColor" /></marker></defs>
          {graph.edges.map(e => <path key={e.from + '>' + e.to} d={e.d} fill="none" stroke="currentColor" strokeWidth={1.5} strokeDasharray={e.to.startsWith('loop:') ? '4 3' : undefined} className={e.to.startsWith('loop:') ? 'text-amber-600' : undefined} markerEnd={`url(#${marker})`} />)}
        </svg>
        {graph.edges.filter(e => e.label).map(e => <span key={'l' + e.from + '>' + e.to} className={`absolute -translate-x-1/2 -translate-y-1/2 whitespace-nowrap rounded-full border bg-card px-1.5 text-[10.5px] ${e.to.startsWith('loop:') ? 'text-amber-600' : 'text-muted-foreground'}`} style={{ left: e.lx, top: e.ly }}>{e.label}</span>)}
        {graph.nodes.map(n => <Node key={n.id} n={n} active={active} onSelect={id => { if (!moved.current) onSelect(id) }} />)}
      </div>
    </div>
    <div className="flex shrink-0 gap-1 pt-1.5">
      <Button variant="outline" size="icon-xs" aria-label="Zoom in" onClick={() => zoom(1.25)}><ZoomIn /></Button>
      <Button variant="outline" size="icon-xs" aria-label="Zoom out" onClick={() => zoom(0.8)}><ZoomOut /></Button>
      <Button variant="outline" size="icon-xs" aria-label="Fit to view" onClick={fit}><Maximize2 /></Button>
    </div>
  </div>
}

function Node({ n, active, onSelect }: { n: Placed; active?: string; onSelect: (id: string) => void }) {
  const Icon = ICON[n.kind]
  const pick = n.kind === 'loop' ? n.target : n.id
  const content = <>
    <span className="grid size-9 shrink-0 place-items-center rounded-lg bg-muted"><Icon className={`size-4 ${TINT[n.kind]}`} /></span>
    <span className="min-w-0 flex-1">
      <span className="flex items-center gap-1.5">
        <span className="truncate text-[13px] font-medium">{n.title}</span>
        {n.status && <span className="ml-auto shrink-0 text-[10.5px]" style={{ color: statusColor(n.status) }}>{n.status}</span>}
      </span>
      {(n.chip || n.note) && <span className="mt-0.5 flex min-w-0 items-center gap-1">
        {n.chip && <span className="max-w-full truncate rounded-full border px-1.5 text-[10.5px] text-muted-foreground">{n.chip}</span>}
        {n.note && <span className="truncate text-[10.5px] text-destructive">{n.note}</span>}
      </span>}
    </span>
  </>
  const card = 'flex h-full w-full items-center gap-2.5 rounded-xl border bg-card px-2.5 text-left shadow-sm'
  return <div className="absolute" style={{ left: n.x, top: n.y, width: NODE_W, height: NODE_H }}>
    {n.kind !== 'start' && <span aria-hidden className="absolute -left-1.5 top-1/2 h-6 w-1 -translate-y-1/2 rounded-full bg-muted-foreground/60" />}
    {pick ? <button type="button" aria-pressed={active === pick} onClick={() => onSelect(pick)}
      className={`${card} cursor-pointer hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring ${active === pick ? 'ring-2 ring-sky-500' : ''} ${n.kind === 'loop' ? 'border-dashed' : ''}`}>{content}</button>
      : <div className={card}>{content}</div>}
  </div>
}
