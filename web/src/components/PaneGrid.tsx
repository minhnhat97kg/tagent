import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from "react"
import { useMedia } from "@/hooks/useMedia"
import { defaultPanes, mergeBoxes, type PaneBox, type PaneId } from "@/lib/paneLayout"
import { cn } from "@/lib/utils"

const ROW = 24
const UNITS = 12
const MIN_H = 4
const IDS: PaneId[] = ["usage", "agents", "tasks"]
const isPane = (id: string): id is PaneId => (IDS as string[]).includes(id)
const STEP: Record<string, [number, number]> = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }
const PHONE: Record<PaneId, string> = { usage: "max-h-[30%] overflow-y-auto", agents: "max-h-[30%]", tasks: "min-h-24 flex-1" }
const same = (a: PaneBox[], b: PaneBox[]) => a.length === b.length && a.every((x, i) => x.id === b[i]!.id && x.x === b[i]!.x && x.y === b[i]!.y && x.w === b[i]!.w && x.h === b[i]!.h)

// PaneGrid is the home view: usage, agents and tasks as boxes on a 12-column grid. On desktop a pane
// is dragged by its header or moved and resized by its handles, and each change is saved. Phones stack the panes, no editing.
export function PaneGrid({ saved, onChange, panes }: { saved?: PaneBox[]; onChange: (boxes: PaneBox[]) => void; panes: Record<PaneId, ReactNode> }) {
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

  if (!desktop) return <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-hidden">
    {IDS.map(id => <div key={id} className={cn("flex min-h-0 flex-col", PHONE[id])}>{panes[id]}</div>)}
  </div>

  const defaults = defaultPanes(rows)
  const boxes = mergeBoxes(saved ?? [], defaults, MIN_H, "")
  // Every change goes through mergeBoxes: the moved box keeps its place, the others move down, unknown ids are kept as they are.
  const place = (base: PaneBox[], id: string, f: (b: PaneBox) => PaneBox) => mergeBoxes(base.map(b => (b.id === id ? f(b) : b)), defaults, MIN_H, id)

  const begin = (id: string, mode: "move" | "resize", e: ReactPointerEvent) => {
    const g = gridRef.current
    if (e.button !== 0 || !g) return
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
    if (!step) return
    e.preventDefault()
    const [dx, dy] = step
    const next = place(boxes, id, b => (mode === "move" ? { ...b, x: b.x + dx, y: b.y + dy } : { ...b, w: b.w + dx, h: b.h + dy }))
    if (!same(next, boxes)) onChange(next)
  }

  const shown = (draft ?? boxes).filter(b => isPane(b.id))
  const cols = Math.max(UNITS, ...shown.map(b => b.x + b.w))
  return <div ref={gridRef} className="grid min-h-0 flex-1 overflow-auto [&_[data-pane-head]]:cursor-move [&_[data-pane-head]]:touch-none"
    style={{ gridTemplateColumns: `repeat(${cols}, calc(100% / ${UNITS}))`, gridAutoRows: "24px" }}>
    {shown.map(b => isPane(b.id) && <div key={b.id} data-pane={b.id} onPointerDown={e => headDown(b.id, e)}
      className="group/pane relative flex min-h-0 min-w-0 select-none flex-col overflow-auto p-1"
      style={{ gridColumn: `${b.x + 1} / span ${b.w}`, gridRow: `${b.y + 1} / span ${b.h}` }}>
      {panes[b.id]}
      <button type="button" data-pane-handle aria-label={`Move ${b.id} pane`} title="Drag to move; arrow keys nudge"
        onPointerDown={e => begin(b.id, "move", e)} onKeyDown={e => nudge(b.id, "move", e)}
        className="absolute left-1/2 top-0 z-10 h-3 w-10 -translate-x-1/2 cursor-move touch-none rounded-b-md bg-muted-foreground/40 opacity-0 group-hover/pane:opacity-100 focus-visible:opacity-100" />
      <button type="button" data-pane-handle aria-label={`Resize ${b.id} pane`} title="Drag to resize; arrow keys nudge"
        onPointerDown={e => begin(b.id, "resize", e)} onKeyDown={e => nudge(b.id, "resize", e)}
        className="absolute bottom-0 right-0 z-10 size-4 cursor-nwse-resize touch-none rounded-br-md border-b-2 border-r-2 border-muted-foreground/60 opacity-0 group-hover/pane:opacity-100 focus-visible:opacity-100" />
    </div>)}
  </div>
}
