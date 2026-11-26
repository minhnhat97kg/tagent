import { Fragment, useCallback, useEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type ReactNode } from "react"
import { createPortal } from "react-dom"
import { Bot, GripVertical, ListTodo, MessageSquare } from "lucide-react"
import { PaneChromeContext } from "@/lib/paneChrome"
import { SplitPane } from "@/components/SplitPane"
import { buttonHolder, cells, DEFAULT_LAYOUT, defaultSizes, dropAt, extName, fits, nodeAt, paneRect, paneTitle, panesOf, reopen, replace, settle, shutIndex, step, type Drop, type Layout, type PaneId, type Path, type Rect } from "@/lib/paneLayout"
import { useMedia } from "@/hooks/useMedia"
import { useSize } from "@/hooks/useSize"
import { cn } from "@/lib/utils"

export type PhoneTab = string // agents, chat, tasks, or a custom pane's id
const TABS = [{ id: "agents", label: "Agents", Icon: Bot }, { id: "chat", label: "Chat", Icon: MessageSquare }, { id: "tasks", label: "Tasks", Icon: ListTodo }]
// A press on one of these in a header is a click or typing, never the start of a drag. The grip is the exception.
const NO_DRAG = "button, a, input, select, textarea, label, summary, [contenteditable], [role=button], [role=tab], [role^=menu]"
const DRAG_PX = 6 // a press that moves less is a click

const SIDES = { above: "Above", below: "Below", left: "Left of", right: "Right of" }
// dropLabel says where a drop lands: beside or in place of a pane, beside a group, or along the whole layout.
function dropLabel({ zone, path }: Drop, layout: Layout, id: PaneId) {
  const names = panesOf(nodeAt(layout, path)).filter(p => p !== id).map(paneTitle).join(" + ")
  if (zone === "swap") return `Swap with ${names}`
  if (!path.length) return zone === "left" || zone === "right" ? `Full height ${zone}` : `Full width ${zone}`
  return `${SIDES[zone]} ${names}`
}

type Props = {
  saved?: Layout
  compact?: boolean // tighter spacing only; the layout is the same
  onChange: (layout: Layout) => void
  tab: PhoneTab
  onTab: (tab: PhoneTab) => void
  panes: Record<PaneId, ReactNode> // the built-in panes and every custom pane, "ext:<name>"
  onRemove?: (id: PaneId) => void // shows each pane's remove button
  tabSlot?: HTMLElement | null // where a phone's tabs render (the app header); above the pane without it
}

// HomeLayout is the home view. On desktop it is a tree of split panes (Desktop). A phone shows one of Agents,
// Chat and Tasks, picked by icon tabs, or a custom pane picked from the header's More menu.
export function HomeLayout(props: Props) {
  const { onTab, panes, tabSlot } = props
  if (useMedia("(min-width: 768px)")) return <Desktop {...props} />

  const custom = extName(props.tab) !== undefined && props.tab in panes
  const tab = custom || TABS.some(t => t.id === props.tab) ? props.tab : "agents" // a deleted pane's tab falls back
  const bar = <div role="tablist" aria-label="Home panes" className="flex shrink-0 gap-0.5 rounded-lg border p-0.5">
    {TABS.map(({ id, label, Icon }) => <button key={id} type="button" role="tab" aria-selected={tab === id} aria-label={label} title={label} onClick={() => onTab(id)}
      className={cn("flex h-7 w-8 items-center justify-center rounded-md", tab === id ? "bg-accent" : "text-muted-foreground")}><Icon className="size-4" /></button>)}
  </div>
  return <div className="flex min-h-0 flex-1 flex-col gap-1">
    {tabSlot === undefined ? bar : tabSlot && createPortal(bar, tabSlot)}
    {tab === "agents" && <>
      <div className="max-h-[30%] min-h-0 overflow-y-auto">{panes.usage}</div>
      <div className="flex min-h-0 flex-1 flex-col">{panes.agents}</div>
    </>}
    {tab === "chat" && <div className="flex min-h-0 flex-1 flex-col">{panes.chat}</div>}
    {tab === "tasks" && <div className="flex min-h-0 flex-1 flex-col">{panes.tasks}</div>}
    {custom && <div key={tab} className="flex min-h-0 flex-1 flex-col">{panes[tab]}</div>}
  </div>
}

// Desktop lays the panes out in nested rows and stacks, split by dividers that are dragged or moved with the
// arrow keys. Dragging a pane's header docks it beside a pane, a group or the whole layout, or swaps two panes;
// arrow keys on its grip move it.
function Desktop({ saved, onChange, onRemove, compact, panes }: Props) {
  const layout = saved ?? DEFAULT_LAYOUT
  const coarse = useMedia("(pointer: coarse)")
  const gap = compact ? (coarse ? 8 : 4) : (coarse ? 12 : 8) // the width SplitPane gives a divider
  const [root, { w, h }] = useSize()
  // Each pane renders into its own element, which moves between places. So a move never remounts a pane:
  // a chat draft and a terminal connection survive it.
  const [hosts] = useState(() => new Map<PaneId, HTMLDivElement>())
  const hostOf = (id: PaneId) => {
    if (!hosts.has(id)) hosts.set(id, Object.assign(document.createElement("div"), { className: "flex min-h-0 min-w-0 flex-1 flex-col" }))
    return hosts.get(id)!
  }
  const [dragging, setDragging] = useState<{ id: PaneId; x: number; y: number; drop?: { label: string; rect: Rect } }>()
  const endDrag = useRef<(() => void) | undefined>(undefined)
  useEffect(() => () => endDrag.current?.(), [])

  // placed is next as the window would show it, or undefined when there is no change or no room for it.
  const placed = (next: Layout | undefined) => {
    const el = root.current!
    const width = el.clientWidth, height = el.clientHeight
    return next && fits(next, layout, width, height, gap) ? { next: settle(next, width, height, gap), width, height } : undefined
  }

  const drag = (id: PaneId, e: PointerEvent<HTMLElement>) => {
    if (e.button !== 0 || endDrag.current) return
    const pressed = e.target as Element
    const control = pressed.closest<HTMLElement>(NO_DRAG)
    // A press in a menu the header opened reaches it through React, but is not inside it.
    if (!e.currentTarget.contains(pressed) || (control && !control.matches("[data-pane-handle]"))) return
    e.preventDefault() // no text selection while dragging
    control?.focus()
    const { pointerId: pointer, clientX: x0, clientY: y0 } = e
    let moving = false
    // What a release here would do; undefined (outside, on itself, no change or no room) does nothing.
    const drop = (ev: globalThis.PointerEvent) => {
      const slot = document.elementFromPoint(ev.clientX, ev.clientY)?.closest<HTMLElement>("[data-pane-slot]")
      if (!slot) return undefined
      const r = slot.getBoundingClientRect()
      const hit = dropAt(layout, id, slot.dataset.paneSlot as PaneId, ev.clientX - r.left, ev.clientY - r.top, r.width, r.height,
        coarse ? 32 : 24, next => !!placed(next))
      const to = hit && placed(hit.next)
      return to && { next: to.next, label: dropLabel(hit, layout, id), rect: paneRect(to.next, id, to.width, to.height, gap) }
    }
    const move = (ev: globalThis.PointerEvent) => {
      if (ev.pointerId !== pointer || (!moving && Math.hypot(ev.clientX - x0, ev.clientY - y0) < DRAG_PX)) return
      moving = true
      document.documentElement.toggleAttribute("data-pane-dragging", true) // the grabbing cursor, everywhere
      setDragging({ id, x: ev.clientX, y: ev.clientY, drop: drop(ev) })
    }
    const up = (ev: globalThis.PointerEvent) => {
      if (ev.pointerId !== pointer) return
      const to = moving ? drop(ev) : undefined
      stop()
      if (to) onChange(to.next)
    }
    const cancel = (ev: globalThis.PointerEvent) => { if (ev.pointerId === pointer) stop() }
    const escape = (ev: globalThis.KeyboardEvent) => {
      if (ev.key !== "Escape" || !moving) return
      ev.preventDefault()
      ev.stopPropagation() // an open ticket stays open
      stop()
    }
    const stop = () => {
      window.removeEventListener("pointermove", move)
      window.removeEventListener("pointerup", up)
      window.removeEventListener("pointercancel", cancel)
      window.removeEventListener("keydown", escape, true)
      document.documentElement.removeAttribute("data-pane-dragging")
      endDrag.current = undefined
      setDragging(undefined)
    }
    window.addEventListener("pointermove", move)
    window.addEventListener("pointerup", up)
    window.addEventListener("pointercancel", cancel)
    window.addEventListener("keydown", escape, true)
    endDrag.current = stop
  }

  const key = (id: PaneId, e: KeyboardEvent) => {
    if (!e.key.startsWith("Arrow")) return
    e.preventDefault()
    const to = placed(step(layout, id, e.key))
    if (to) onChange(to.next)
  }

  const render = (node: Layout, path: Path, area: Rect): ReactNode => {
    if (typeof node === "string") return <Position key={node} id={node} host={hostOf(node)} from={dragging?.id === node} />
    const list = cells(node, area, gap)
    const shut = shutIndex(node)
    const open = node.children.flatMap((_, i) => (i === shut ? [] : [i]))
    const usage = node.axis === "y" ? open.findIndex(i => node.children[i] === "usage") : -1
    const first = (i: number) => paneTitle(panesOf(node.children[i]!)[0]!)
    // A shut Usage has no height, so its reopen button sits where it was, on a neighbour.
    const button = (where: string) => <button type="button" aria-label="Show usage" title="Show usage" onClick={() => onChange(reopen(layout))}
      className={cn("h-8 shrink-0 px-2.5 text-left text-[12.5px] font-semibold hover:bg-accent", where)}>{paneTitle("usage")}</button>
    return <SplitPane key={panesOf(node).join()} axis={node.axis} compact={compact} sizes={open.map(i => node.sizes[i]!)} defaults={defaultSizes(node.axis, open.length)}
      mins={open.map((i, k) => (k === usage ? 0 : list[i]!.min))} collapsible={usage < 0 ? undefined : usage}
      labels={open.slice(1).map((i, k) => `${first(open[k]!)} ${node.axis === "x" ? "and" : "over"} ${first(i)}`)}
      onChange={next => onChange(replace(layout, path, { ...node, sizes: node.children.map((_, i) => (i === shut ? 0 : next[open.indexOf(i)]!)) }))}>
      {open.map(i => {
        const holds = buttonHolder(shut) === i
        return <Fragment key={panesOf(node.children[i]!).join()}>
          {holds && shut < i && button("border-b")}
          {render(node.children[i]!, [...path, i], list[i]!.rect)}
          {holds && shut > i && button("border-t")}
        </Fragment>
      })}
    </SplitPane>
  }

  return <>
    <div ref={root} className="relative flex min-h-0 min-w-0 flex-1">
      {render(layout, [], { left: 0, top: 0, width: w, height: h })}
      {/* The area the dragged pane would take. */}
      {dragging?.drop && <div data-drop style={dragging.drop.rect} className="pointer-events-none absolute z-30 flex items-center justify-center rounded-lg bg-brand/10 ring-2 ring-brand ring-inset">
        <span className="rounded-md border bg-popover px-2.5 py-1 text-xs font-semibold whitespace-nowrap text-popover-foreground shadow-md">{dragging.drop.label}</span>
      </div>}
    </div>
    {/* After the split, so each element is in the document before its pane's effects run. */}
    {panesOf(layout).map((id, _, all) => {
      const single = all.length < 2 // the last pane stays
      const grip = <button type="button" data-pane-handle aria-label={`Move ${id} pane`} title="Drag to move, or use the arrow keys" onKeyDown={e => key(id, e)}
        className="-ml-1.5 flex h-6 w-5 shrink-0 cursor-grab items-center justify-center rounded text-muted-foreground hover:bg-accent hover:text-foreground pointer-coarse:h-8">
        <GripVertical className="size-3.5" />
      </button>
      return createPortal(<PaneChromeContext.Provider value={{ id, grip, onPointerDown: e => drag(id, e), onRemove: single || !onRemove ? undefined : () => onRemove(id) }}>
        {id === "tasks" ? panes.tasks : <Slot id={id}>{panes[id]}</Slot>}
      </PaneChromeContext.Provider>, hostOf(id), id)
    })}
    {/* Above the pointer, so a finger does not cover it. */}
    {dragging && <div aria-hidden data-pane-chip style={{ left: dragging.x + 12, top: Math.max(4, dragging.y - 40) }}
      className="pointer-events-none fixed z-50 rounded-md border bg-popover px-2 py-1 text-xs font-semibold text-popover-foreground shadow-md">{paneTitle(dragging.id)}</div>}
  </>
}

// Where focus and scrolling were when a pane's element left its position: the browser forgets both.
const left = new WeakMap<HTMLElement, () => void>()

// Position is a leaf of the tree; it holds the element of the pane that sits there.
function Position({ id, host, from }: { id: PaneId; host: HTMLElement; from: boolean }) {
  const ref = useCallback((el: HTMLDivElement | null) => {
    if (!el) return
    el.appendChild(host)
    left.get(host)?.()
    return () => {
      const focus = host.contains(document.activeElement) ? document.activeElement as HTMLElement : undefined
      const scrolled = Array.from(host.querySelectorAll("*")).filter(n => n.scrollTop || n.scrollLeft).map(n => [n, n.scrollTop, n.scrollLeft] as const)
      left.set(host, () => {
        for (const [n, y, x] of scrolled) { n.scrollTop = y; n.scrollLeft = x }
        focus?.focus({ preventScroll: true })
      })
      host.remove()
    }
  }, [host])
  return <div ref={ref} data-pane-slot={id} className={cn("flex min-h-0 min-w-0 flex-1 flex-col", from && "opacity-60")} />
}

// Slot names one desktop pane; hovering it shows its maximize link.
function Slot({ id, children }: { id: string; children: ReactNode }) {
  return <div data-pane={id} className="group/pane flex min-h-0 min-w-0 flex-1 flex-col">{children}</div>
}
