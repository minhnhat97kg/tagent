import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from "react"
import { useMedia } from "@/hooks/useMedia"
import { Maximize2 } from "lucide-react"
import { compactRows, defaultPanes, mergeBoxes, packBoxes, type PaneBox, type PaneId } from "@/lib/paneLayout"
import { cn } from "@/lib/utils"

const ROW = 24
const UNITS = 12
const MIN_H = 4
const IDS: PaneId[] = ["usage", "agents", "workflows", "tasks"]
const isPane = (id: string): id is PaneId => (IDS as string[]).includes(id)
const STEP: Record<string, [number, number]> = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }
// A phone stacks these three; the workflows pane is desktop only.
const PHONE: Record<Exclude<PaneId, "workflows">, string> = { usage: "max-h-[30%] overflow-y-auto", agents: "max-h-[30%]", tasks: "min-h-24 flex-1" }
const same = (a: PaneBox[], b: PaneBox[]) => a.length === b.length && a.every((x, i) => x.id === b[i]!.id && x.x === b[i]!.x && x.y === b[i]!.y && x.w === b[i]!.w && x.h === b[i]!.h)

// PaneGrid is the home view: usage, agents and tasks as boxes on a 12-column grid. On desktop a pane
// is dragged by its header or moved and resized by its handles, and each change is saved. Phones stack the panes, no editing.
// compact draws the panes packed at their small heights; the saved layout is kept and cannot be edited.
export function PaneGrid({ saved, onChange, panes, compact }: { saved?: PaneBox[]; onChange: (boxes: PaneBox[]) => void; panes: Record<PaneId, ReactNode>; compact?: boolean }) {
  const desktop = useMedia("(min-width: 768px)")
  const gridRef = useRef<HTMLDivElement>(null)
  const [rows, setRows] = useState(24)
  const [draft, setDraft] = useState<PaneBox[]>()
  const stop = useRef<(() => void) | undefined>(undefined) // ends a drag in progress

  useEffect(() => {
    const el = gridRef.current
    if (!el) return
    const measure = () => setRows(Math.max(24, Math.floor(el.clientHeight / ROW)))
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    measure()
    return () => ro.disconnect()
  }, [desktop])
  useEffect(() => () => stop.current?.(), [])
  useEffect(() => { if (compact) stop.current?.() }, [compact])

  if (!desktop) return <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-hidden">
    {(Object.keys(PHONE) as (keyof typeof PHONE)[]).map(id => <div key={id} className={cn("flex min-h-0 flex-col", PHONE[id])}>{panes[id]}</div>)}
  </div>

  const defaults = defaultPanes(rows)
  const merged = mergeBoxes(saved ?? [], defaults, MIN_H, "")
  const boxes = compact ? packBoxes(merged.filter(b => isPane(b.id)), compactRows) : merged
  // Every change goes through mergeBoxes: the moved box keeps its place, the others move down, unknown ids are kept as they are.
  const place = (base: PaneBox[], id: string, f: (b: PaneBox) => PaneBox) => mergeBoxes(base.map(b => (b.id === id ? f(b) : b)), defaults, MIN_H, id)

  const begin = (id: string, mode: "move" | "resize", e: ReactPointerEvent) => {
    const g = gridRef.current
    if (compact || e.button !== 0 || !g) return
    const start = boxes
    const unit = g.clientWidth / UNITS || 1
    const x0 = e.clientX, y0 = e.clientY, left0 = g.scrollLeft, top0 = g.scrollTop
    let last = start
    const move = (ev: PointerEvent) => {
      const dx = Math.round((ev.clientX - x0 + g.scrollLeft - left0) / unit)
      const dy = Math.round((ev.clientY - y0 + g.scrollTop - top0) / ROW)
      last = place(start, id, b => (mode === "move" ? { ...b, x: b.x + dx, y: b.y + dy } : { ...b, w: b.w + dx, h: b.h + dy }))
      setDraft(last)
    }
    function end() {
      window.removeEventListener("pointermove", move)
      window.removeEventListener("pointerup", up)
      window.removeEventListener("pointercancel", end)
      stop.current = undefined
      setDraft(undefined)
    }
    function up() {
      end()
      if (!same(last, start)) onChange(last) // a plain click changes nothing, so the default is never frozen
    }
    window.addEventListener("pointermove", move)
    window.addEventListener("pointerup", up)
    window.addEventListener("pointercancel", end)
    stop.current = end
  }

  // A press on the header starts a move, but not on a control in it.
  const headDown = (id: string, e: ReactPointerEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement
    const head = target.closest("[data-pane-head]")
    if (!head || head !== e.currentTarget.querySelector("[data-pane-head]") || target.closest("button,input,select,textarea,a,label")) return
    begin(id, "move", e)
  }

  const nudge = (id: string, mode: "move" | "resize", e: ReactKeyboardEvent) => {
    const step = STEP[e.key]
    if (!step || compact) return
    e.preventDefault()
    const [dx, dy] = step
    const next = place(boxes, id, b => (mode === "move" ? { ...b, x: b.x + dx, y: b.y + dy } : { ...b, w: b.w + dx, h: b.h + dy }))
    if (!same(next, boxes)) onChange(next)
  }

  const shown = (draft ?? boxes).filter(b => isPane(b.id))
  const cols = Math.max(UNITS, ...shown.map(b => b.x + b.w))
  return <div ref={gridRef} className={cn("grid min-h-0 flex-1 overflow-auto", !compact && "[&_[data-pane-head]]:cursor-move [&_[data-pane-head]]:touch-none")}
    style={{ gridTemplateColumns: `repeat(${cols}, calc(100% / ${UNITS}))`, gridAutoRows: "24px" }}>
    {shown.map(b => isPane(b.id) && <div key={b.id} data-pane={b.id} onPointerDown={compact ? undefined : e => headDown(b.id, e)}
      className="group/pane relative flex min-h-0 min-w-0 select-none flex-col overflow-auto p-1"
      style={{ gridColumn: `${b.x + 1} / span ${b.w}`, gridRow: `${b.y + 1} / span ${b.h}` }}>
      {panes[b.id]}
      {!compact && <>
        <button type="button" data-pane-handle aria-label={`Move ${b.id} pane`} title="Drag to move; arrow keys nudge"
          onPointerDown={e => begin(b.id, "move", e)} onKeyDown={e => nudge(b.id, "move", e)}
          className="absolute left-1/2 top-0 z-10 h-3 w-10 -translate-x-1/2 cursor-move touch-none rounded-b-md bg-muted-foreground/40 opacity-0 group-hover/pane:opacity-100 focus-visible:opacity-100" />
        <button type="button" data-pane-handle aria-label={`Resize ${b.id} pane`} title="Drag to resize; arrow keys nudge"
          onPointerDown={e => begin(b.id, "resize", e)} onKeyDown={e => nudge(b.id, "resize", e)}
          className="absolute bottom-0 right-0 z-10 size-4 cursor-nwse-resize touch-none rounded-br-md border-b-2 border-r-2 border-muted-foreground/60 opacity-0 group-hover/pane:opacity-100 focus-visible:opacity-100" />
      </>}
      <a href={`#/pane/${b.id}`} data-pane-handle aria-label={`Maximize ${b.id} pane`} title="Maximize"
        className="absolute bottom-0 left-0 z-10 flex size-4 items-center justify-center rounded-tr-md text-muted-foreground hover:text-foreground">
        <Maximize2 className="size-3" />
      </a>
    </div>)}
  </div>
}
