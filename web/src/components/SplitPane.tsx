import { Children, Fragment, useEffect, useRef, useState, type CSSProperties, type KeyboardEvent, type PointerEvent, type ReactNode } from "react"
import { cn } from "@/lib/utils"

// A collapsible pane below this many px is shut (0); a shut pane opens to at least this much.
export const COLLAPSE_PX = 64
// Keys move a divider by this fraction of the container.
const STEP = 0.02

// moveDivider moves the divider between pane i and pane i+1 by px pixels in a container total px long.
// sizes are fractions that sum to 1; only panes i and i+1 change. mins are px. A move that would break a
// min, or a container too small for both mins, returns sizes unchanged (the same array).
export function moveDivider(sizes: number[], i: number, px: number, total: number, mins: number[], collapsible?: number): number[] {
  if (total <= 0 || i < 0 || i + 1 >= sizes.length) return sizes
  const a = sizes[i]! * total
  const b = sizes[i + 1]! * total
  const sum = a + b
  const min = (k: number) => (k === collapsible ? 0 : (mins[k] ?? 0))
  if (sum < min(i) + min(i + 1)) return sizes
  let va = Math.min(Math.max(a + px, min(i)), sum - min(i + 1))
  // The collapsible pane shuts when dragged below COLLAPSE_PX, and a shut pane opens to at least COLLAPSE_PX.
  const snap = (v: number, cur: number) => (v < COLLAPSE_PX ? (v > cur ? Math.min(COLLAPSE_PX, sum) : 0) : v)
  if (collapsible === i) va = snap(va, a)
  else if (collapsible === i + 1) va = sum - snap(sum - va, b)
  if (va === a || va < min(i) || sum - va < min(i + 1)) return sizes
  const out = sizes.slice()
  out[i] = va / total
  out[i + 1] = (sum - va) / total
  return out
}

// SplitPane lays its children out in a row (axis x, columns) or a column (axis y), with a divider between each pair.
// A divider drags with the pointer, moves with the arrow keys when focused, and resets to defaults on double-click.
// Sizes are fractions; onChange gets the new fractions once per drag, at pointer up, and at once for a key or a reset.
export function SplitPane({ axis, sizes, defaults, mins, collapsible, labels, compact, onChange, className, children }: {
  axis: "x" | "y"; sizes: number[]; defaults: number[]; mins: number[]; collapsible?: number; labels: string[]; compact?: boolean
  onChange: (sizes: number[]) => void; className?: string; children: ReactNode[]
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [draft, setDraft] = useState<number[]>()
  const [dragging, setDragging] = useState<number>()
  const endDrag = useRef<(() => void) | undefined>(undefined) // ends a drag in progress
  useEffect(() => () => endDrag.current?.(), [])
  const shown = draft ?? sizes
  const size = () => (!ref.current ? 0 : axis === "x" ? ref.current.clientWidth : ref.current.clientHeight)
  const at = (e: { clientX: number; clientY: number }) => (axis === "x" ? e.clientX : e.clientY)

  const drag = (i: number, e: PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return
    e.preventDefault() // no text selection while dragging
    e.currentTarget.focus()
    const start = sizes
    const total = size()
    const p0 = at(e)
    let last = start
    const move = (ev: globalThis.PointerEvent) => {
      last = moveDivider(start, i, at(ev) - p0, total, mins, collapsible)
      setDraft(last)
    }
    const stop = () => {
      window.removeEventListener("pointermove", move)
      window.removeEventListener("pointerup", up)
      window.removeEventListener("pointercancel", stop)
      endDrag.current = undefined
      setDraft(undefined)
      setDragging(undefined)
    }
    const up = () => {
      const changed = last.some((v, k) => v !== start[k])
      stop()
      if (changed) onChange(last)
    }
    window.addEventListener("pointermove", move)
    window.addEventListener("pointerup", up)
    window.addEventListener("pointercancel", stop)
    endDrag.current = stop
    setDragging(i)
  }

  const key = (i: number, e: KeyboardEvent<HTMLDivElement>) => {
    const back = axis === "x" ? "ArrowLeft" : "ArrowUp"
    const fwd = axis === "x" ? "ArrowRight" : "ArrowDown"
    if (e.key !== back && e.key !== fwd) return
    e.preventDefault()
    const total = size()
    const next = moveDivider(sizes, i, (e.key === fwd ? 1 : -1) * STEP * total, total, mins, collapsible)
    if (next !== sizes) onChange(next)
  }

  const kids = Children.toArray(children) // null/false children are not panes, so they get no divider
  return <div ref={ref} className={cn("flex min-h-0 min-w-0 flex-1", axis === "x" ? "flex-row" : "flex-col", className)}>
    {kids.map((child, k) => {
      const shut = k === collapsible && shown[k] === 0
      const style: CSSProperties = shut
        ? { flex: "0 0 0px", overflow: "hidden", [axis === "x" ? "minWidth" : "minHeight"]: 0 }
        : { flex: `${shown[k]} 1 0%`, [axis === "x" ? "minWidth" : "minHeight"]: mins[k] ?? 0 }
      return <Fragment key={k}>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden" style={style}>{child}</div>
        {k < kids.length - 1 && <div role="separator" tabIndex={0} aria-label={labels[k]} aria-orientation={axis === "x" ? "vertical" : "horizontal"}
          aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(shown[k]! * 100)} data-pane-handle data-split-handle
          onPointerDown={e => drag(k, e)} onDoubleClick={() => onChange(defaults)} onKeyDown={e => key(k, e)}
          data-dragging={dragging === k || undefined}
          // The grip shows only on hover, keyboard focus or a drag. The ::before is the hit area: on touch it reaches 8px into both panes, so a finger can grab it.
          className={cn("group relative z-10 flex shrink-0 touch-none select-none items-center justify-center outline-none [-webkit-touch-callout:none] before:absolute before:inset-0",
            axis === "x" ? cn("cursor-col-resize pointer-coarse:before:-inset-x-2", compact ? "w-1 pointer-coarse:w-2" : "w-2 pointer-coarse:w-3")
              : cn("cursor-row-resize pointer-coarse:before:-inset-y-2", compact ? "h-1 pointer-coarse:h-2" : "h-2 pointer-coarse:h-3"))}>
          <span aria-hidden className={cn("pointer-events-none rounded-full bg-brand opacity-0 transition-all group-hover:opacity-100 group-focus-visible:opacity-100 group-active:opacity-100 group-data-dragging:opacity-100",
            axis === "x" ? "h-9 w-1 group-hover:h-14 group-focus-visible:h-14 group-data-dragging:h-20 pointer-coarse:h-12"
              : "h-1 w-9 group-hover:w-14 group-focus-visible:w-14 group-data-dragging:w-20 pointer-coarse:w-12")} />
        </div>}
      </Fragment>
    })}
  </div>
}
