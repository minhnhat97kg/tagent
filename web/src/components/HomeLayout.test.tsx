import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { HomeLayout } from "./HomeLayout"
import { moveDivider } from "./SplitPane"
import { DEFAULT_LAYOUT, shape, type Layout, type PaneId, type Split } from "@/lib/paneLayout"
import { Panel } from "./Panel"
import { useEffect, useState } from "react"
import { createPortal } from "react-dom"

// A pointer event as the split reads it: clientX/clientY and button.
const ptr = (type: string, x = 0, y = 0) => Object.assign(new Event(type, { bubbles: true, cancelable: true }), { clientX: x, clientY: y, button: 0 })
const panes = () => ({ agents: <p>Agents pane</p>, usage: <p>Usage pane</p>, chat: <p>Chat pane</p>, tasks: <p>Tasks pane</p> })
const sep = (name: string) => screen.getByRole("separator", { name })
const sum = (a: number[]) => a.reduce((s, v) => s + v, 0)

// The container is 1000px wide and 800px tall.
beforeEach(() => {
  globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver
  vi.spyOn(Element.prototype, "clientWidth", "get").mockReturnValue(1000)
  vi.spyOn(Element.prototype, "clientHeight", "get").mockReturnValue(800)
  window.matchMedia = ((q: string) => ({ matches: true, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
})
afterEach(() => { cleanup(); vi.restoreAllMocks() })

// The default layout with its column widths and left heights replaced.
const sized = (cols: number[], left = [0.65, 0.35]): Split => ({ ...DEFAULT_LAYOUT, sizes: cols, children: [{ ...(DEFAULT_LAYOUT.children[0] as Split), sizes: left }, "chat", "tasks"] })
const widths = (l: Layout) => (l as Split).sizes
const heights = (l: Layout) => ((l as Split).children[0] as Split).sizes
const DEFAULT_WIDTHS = DEFAULT_LAYOUT.sizes
const view = (onChange = vi.fn(), saved?: Layout) => {
  render(<HomeLayout saved={saved} onChange={onChange} tab="agents" onTab={() => {}} panes={panes()} />)
  return onChange
}

test("dragging the Agents and Chat divider changes the columns and saves once, with fractions that add up to 1", () => {
  const onChange = view()
  const divider = sep("Agents and Chat")
  fireEvent(divider, ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", 50, 0))
  fireEvent(window, ptr("pointermove", 100, 0))
  expect(onChange).not.toHaveBeenCalled() // saved on release, not while dragging
  fireEvent(window, ptr("pointerup", 100, 0))
  expect(onChange).toHaveBeenCalledTimes(1)
  const next = onChange.mock.calls[0]![0] as Layout
  expect(widths(next)[0]! * 1000).toBeCloseTo(0.22 * 1000 + 100)
  expect(sum(widths(next))).toBeCloseTo(1)
  expect(heights(next)).toEqual([0.65, 0.35])
})

test("a drag cannot squeeze Agents below its minimum, and a cancelled drag saves nothing", () => {
  const onChange = view(vi.fn(), sized([0.3, 0.4, 0.3]))
  const divider = sep("Agents and Chat")
  fireEvent(divider, ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", -900, 0))
  fireEvent(window, ptr("pointerup", -900, 0))
  expect(widths(onChange.mock.calls[0]![0] as Layout)[0]! * 1000).toBeCloseTo(220)

  fireEvent(divider, ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", 200, 0))
  fireEvent(window, ptr("pointercancel", 200, 0))
  expect(onChange).toHaveBeenCalledTimes(1)
})

test("arrow keys on a focused divider move it by 2% and save at once; double-click resets", () => {
  const onChange = view(vi.fn(), sized([0.3, 0.4, 0.3], [0.5, 0.5]))
  const divider = sep("Agents and Chat")
  expect(divider.getAttribute("aria-valuenow")).toBe("30")
  expect(divider.getAttribute("aria-orientation")).toBe("vertical")
  fireEvent.keyDown(divider, { key: "ArrowRight" })
  expect(widths(onChange.mock.calls[0]![0] as Layout)[0]).toBeCloseTo(0.32)
  fireEvent.keyDown(divider, { key: "ArrowDown" }) // the other axis does nothing here
  expect(onChange).toHaveBeenCalledTimes(1)
  fireEvent.doubleClick(divider)
  expect(onChange.mock.calls[1]![0]).toEqual(sized(DEFAULT_WIDTHS, [0.5, 0.5]))
})

test("the Agents over Usage divider is horizontal; its arrow keys move it, and dragging it down collapses Usage", () => {
  const onChange = view()
  const divider = sep("Agents over Usage")
  expect(divider.getAttribute("aria-orientation")).toBe("horizontal")
  fireEvent.keyDown(divider, { key: "ArrowUp" })
  expect(heights(onChange.mock.calls[0]![0] as Layout)[1]).toBeCloseTo(0.37)

  fireEvent(divider, ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", 0, 600))
  fireEvent(window, ptr("pointerup", 0, 600))
  expect(heights(onChange.mock.calls[1]![0] as Layout)).toEqual([1, 0])
})

test("a collapsed Usage shows a button that reopens it at the default size", () => {
  const onChange = view(vi.fn(), sized(DEFAULT_WIDTHS, [1, 0]))
  expect(screen.queryByText("Usage pane")).toBeNull()
  expect(screen.queryByRole("separator", { name: "Agents over Usage" })).toBeNull() // no divider to a shut pane
  fireEvent.click(screen.getByRole("button", { name: "Show usage" }))
  expect(onChange).toHaveBeenCalledWith(DEFAULT_LAYOUT)
})

test("a shut pane opens at 64px on a small step, and an open one shuts below 64px", () => {
  expect(moveDivider([0.65, 0], 0, -2, 800, [128, 0], 1)[1]! * 800).toBeCloseTo(64)
  expect(moveDivider([0.65, 0.35], 0, 0, 800, [128, 0], 1)).toEqual([0.65, 0.35])
  expect(moveDivider([0.92, 0.08], 0, 10, 800, [128, 0], 1)[1]).toBe(0)
})

test("each desktop pane header links to its maximized page, at its right end", () => {
  render(<HomeLayout onChange={vi.fn()} tab="agents" onTab={() => {}} panes={titled()} />)
  expect(screen.getByRole("link", { name: "Maximize agents pane" }).getAttribute("href")).toBe("#/pane/agents")
  const chat = screen.getByRole("link", { name: "Maximize chat pane" })
  expect(chat.getAttribute("href")).toBe("#/pane/chat")
  expect(chat.closest("[data-pane-head]")!.lastElementChild).toBe(chat)
  expect(screen.getAllByRole("link", { name: /^Maximize/ })).toHaveLength(4)
})

test("a phone shows one tab at a time, with the Agents tab stacking Usage over the agents", () => {
  window.matchMedia = ((q: string) => ({ matches: false, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
  const onTab = vi.fn()
  render(<HomeLayout onChange={vi.fn()} tab="agents" onTab={onTab} panes={panes()} />)
  expect(screen.getByText("Usage pane")).toBeTruthy()
  expect(screen.getByText("Agents pane")).toBeTruthy()
  expect(screen.queryByText("Chat pane")).toBeNull()
  fireEvent.click(screen.getByRole("tab", { name: "Chat" }))
  expect(onTab).toHaveBeenCalledWith("chat")
  expect(screen.queryByRole("separator")).toBeNull()
})

test("a phone pane header has no grip, maximize link or drag handle", () => {
  window.matchMedia = ((q: string) => ({ matches: false, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
  render(<HomeLayout onChange={vi.fn()} tab="agents" onTab={() => {}} panes={titled()} />)
  expect(screen.queryByRole("link")).toBeNull()
  expect(screen.queryByRole("button", { name: /^Move/ })).toBeNull()
  expect(document.querySelector("[data-pane-head]")!.className).not.toContain("touch-none")
})

// A pane with a draft, which counts its mounts.
let mounts = 0
function Draft() {
  const [text, setText] = useState("")
  useEffect(() => { mounts++ }, [])
  return <Panel title="Chat"><textarea aria-label="draft" value={text} onChange={e => setText(e.target.value)} /></Panel>
}
const titled = () => ({ agents: <Panel title="Agents">a</Panel>, usage: <Panel title="Usage">u</Panel>, chat: <Draft />, tasks: <Panel title="Tasks">t</Panel> })
// The layout saves into its own state, as App does.
function Live({ onChange, first }: { onChange: (l: Layout) => void; first?: Layout }) {
  const [saved, setSaved] = useState(first)
  return <HomeLayout saved={saved} onChange={l => { setSaved(l); onChange(l) }} tab="agents" onTab={() => {}} panes={titled()} />
}
const grip = (id: PaneId) => screen.getByRole("button", { name: `Move ${id} pane` })
const slot = (id: PaneId) => document.querySelector<HTMLElement>(`[data-pane-slot="${id}"]`)!
// The arrangement on screen: a split is x(...) or y(...) by the direction of its element.
const shown = (el: Element = document.querySelector(".flex-row, .flex-col:has(> [role=separator])")!): string => {
  const cells = Array.from(el.querySelectorAll(":scope > div:not([role=separator])"))
  const child = (c: Element) => {
    const inner = c.querySelector(":scope > [data-pane-slot]")
    return inner ? inner.getAttribute("data-pane-slot")! : shown(c.querySelector(":scope > div")!)
  }
  return `${el.classList.contains("flex-row") ? "x" : "y"}(${cells.map(child).join(" ")})`
}
const heading = (name: string) => screen.getByRole("heading", { name })
const drop = () => document.querySelector<HTMLElement>("[data-drop]")
// Every pane is a 100x100 box, so a pointer position is a share of the pane under it.
const under = (el: Element | null, size = { width: 100, height: 100 }) => {
  document.elementFromPoint = () => el
  vi.spyOn(Element.prototype, "getBoundingClientRect").mockReturnValue({ left: 0, top: 0, ...size } as DOMRect)
}
// Drags the pane by its grip over the target and holds it there.
const over = (id: PaneId, target: PaneId, x: number, y: number, size?: { width: number; height: number }) => {
  under(slot(target), size)
  fireEvent(grip(id), ptr("pointerdown", 500, 500))
  fireEvent(window, ptr("pointermove", x, y))
}
const release = (x: number, y: number) => fireEvent(window, ptr("pointerup", x, y))

test.each([
  ["top", "agents", 50, 30, "Above Agents", "x(y(chat agents usage) tasks)"],
  ["bottom", "agents", 50, 90, "Below Agents", "x(y(agents chat usage) tasks)"],
  ["left", "agents", 20, 50, "Left of Agents", "x(y(x(chat agents) usage) tasks)"],
  ["right", "tasks", 80, 50, "Right of Tasks", "x(y(agents usage) tasks chat)"],
  ["middle", "agents", 50, 50, "Swap with Agents", "x(y(chat usage) agents tasks)"],
] as const)("the %s of a pane docks there, and the drop keeps the pane mounted with its draft", (_, target, x, y, label, arrangement) => {
  mounts = 0
  const onChange = vi.fn()
  render(<Live onChange={onChange} />)
  fireEvent.change(screen.getByLabelText("draft"), { target: { value: "half typed" } })
  // A pane's own edge is inside the strips for the larger groups at that side.
  over("chat", target, x, y)
  expect(drop()!.textContent).toBe(label)
  release(x, y)
  expect(shape(onChange.mock.calls[0]![0] as Layout)).toBe(arrangement)
  expect(shown()).toBe(arrangement)
  expect((screen.getByLabelText("draft") as HTMLTextAreaElement).value).toBe("half typed")
  expect(mounts).toBe(1)
  expect(drop()).toBeNull()
  expect(onChange).toHaveBeenCalledTimes(1)
})

test("a strip along the page's edge spans the full width, and the highlight is the area the pane will take", () => {
  vi.spyOn(Element.prototype, "clientWidth", "get").mockReturnValue(2024) // wide enough that no minimum applies
  const onChange = vi.fn()
  render(<Live onChange={onChange} />)
  over("tasks", "chat", 50, 99) // the bottom edge, with the 12px dividers of a coarse pointer
  expect(drop()!.textContent).toBe("Full width below")
  const { left, top, width, height } = drop()!.style
  ;[left, top, width, height].map(parseFloat).forEach((v, i) => expect(v).toBeCloseTo([0, 394 + 12, 2024, 394][i]!))
  release(50, 99)
  expect(shown()).toBe("y(x(y(agents usage) chat) tasks)")
  expect(sep("Agents over Tasks")).toBeTruthy()
  expect(sep("Agents and Chat")).toBeTruthy()
  expect(sep("Agents over Usage")).toBeTruthy()
})

test("nested splits come and go without remounting a pane, and every divider works", () => {
  mounts = 0
  const onChange = vi.fn()
  render(<Live onChange={onChange} />)
  fireEvent.change(screen.getByLabelText("draft"), { target: { value: "kept" } })
  for (const [id, key] of [["tasks", "ArrowDown"], ["usage", "ArrowRight"], ["usage", "ArrowRight"], ["usage", "ArrowRight"]] as const) fireEvent.keyDown(grip(id), { key })
  expect(shown()).toBe("x(y(x(agents chat) tasks) usage)")
  expect(screen.getAllByRole("separator")).toHaveLength(3)
  fireEvent.keyDown(sep("Agents and Chat"), { key: "ArrowRight" })
  const last = onChange.mock.lastCall![0] as Split & { children: [Split & { children: [Split] }] }
  expect(last.children[0].children[0].sizes[0]).toBeGreaterThan(0.2)
  expect(onChange).toHaveBeenCalledTimes(5)
  expect((screen.getByLabelText("draft") as HTMLTextAreaElement).value).toBe("kept")
  expect(mounts).toBe(1)
})

test("a place with no room is not offered, and a drop that changes nothing does nothing", () => {
  vi.spyOn(Element.prototype, "clientWidth", "get").mockReturnValue(900) // four columns need 976px
  const onChange = vi.fn()
  render(<Live onChange={onChange} />)
  for (const [id, target, x, y] of [["usage", "chat", 80, 50], ["usage", "agents", 50, 90], ["chat", "chat", 50, 50]] as const) {
    over(id, target, x, y)
    expect(drop()?.textContent ?? null, `${id} on ${target}`).toBeNull()
    release(x, y)
  }
  fireEvent.keyDown(grip("usage"), { key: "ArrowLeft" }) // a fourth column
  expect(onChange).not.toHaveBeenCalled()
})

test.each([
  ["Escape", () => fireEvent.keyDown(window, { key: "Escape" })],
  ["a cancelled pointer", () => fireEvent(window, ptr("pointercancel"))],
])("%s cancels a drag", (_, cancel) => {
  const onChange = vi.fn()
  render(<Live onChange={onChange} />)
  over("chat", "agents", 50, 50)
  expect(drop()).toBeTruthy()
  cancel()
  expect(drop()).toBeNull()
  expect(document.querySelector("[data-pane-chip]")).toBeNull()
  expect(document.documentElement.hasAttribute("data-pane-dragging")).toBe(false)
  release(50, 50)
  expect(onChange).not.toHaveBeenCalled()
})

test("dropping outside every pane changes nothing", () => {
  const onChange = vi.fn()
  render(<Live onChange={onChange} />)
  for (const el of [null, document.body]) {
    under(el)
    fireEvent(grip("chat"), ptr("pointerdown"))
    fireEvent(window, ptr("pointermove", 10, 10))
    release(10, 10)
  }
  expect(onChange).not.toHaveBeenCalled()
})

test("a header is a drag handle: past 6px a chip names the pane, and a drop docks it", () => {
  const onChange = vi.fn()
  render(<Live onChange={onChange} />)
  expect(heading("Chat").closest("[data-pane-head]")!.className).toContain("touch-none")
  under(slot("agents"))
  fireEvent(heading("Chat"), ptr("pointerdown", 50, 45))
  fireEvent(window, ptr("pointermove", 53, 49)) // 5px: still a click
  expect(document.querySelector("[data-pane-chip]")).toBeNull()
  expect(drop()).toBeNull()
  fireEvent(window, ptr("pointermove", 50, 55))
  expect(document.querySelector("[data-pane-chip]")!.textContent).toBe("Chat")
  expect(document.documentElement.hasAttribute("data-pane-dragging")).toBe(true)
  release(50, 55)
  expect(shown()).toBe("x(y(chat usage) agents tasks)")
  expect(document.querySelector("[data-pane-chip]")).toBeNull()
  expect(document.documentElement.hasAttribute("data-pane-dragging")).toBe(false)
})

test("a press that stays within 6px changes nothing", () => {
  const onChange = vi.fn()
  render(<Live onChange={onChange} />)
  under(slot("agents"))
  fireEvent(heading("Chat"), ptr("pointerdown"))
  fireEvent(window, ptr("pointermove", 3, 3))
  release(3, 3)
  expect(onChange).not.toHaveBeenCalled()
})

test("a drag never starts from a control in a header, or from a menu the header opened", () => {
  const onChange = vi.fn()
  const controls = <>
    <button>a button</button><a href="#x">a link</a><input aria-label="a field" /><span role="tab">a tab</span><div role="menu"><span>in a menu</span></div>
    {createPortal(<span>portaled</span>, document.body)}
  </>
  render(<HomeLayout onChange={onChange} tab="agents" onTab={() => {}} panes={{ ...titled(), chat: <Panel title="Chat" right={controls}>c</Panel> }} />)
  under(slot("agents"))
  for (const el of [screen.getByText("a button"), screen.getByText("a link"), screen.getByLabelText("a field"), screen.getByText("a tab"), screen.getByText("in a menu"), screen.getByText("portaled"), screen.getByRole("link", { name: "Maximize chat pane" })]) {
    fireEvent(el, ptr("pointerdown"))
    fireEvent(window, ptr("pointermove", 50, 50))
    expect(document.querySelector("[data-pane-chip]"), el.textContent ?? "").toBeNull()
    release(50, 50)
  }
  expect(onChange).not.toHaveBeenCalled()
})

test("arrow keys on a focused grip move the pane, and focus follows it", () => {
  const onChange = vi.fn()
  render(<Live onChange={onChange} />)
  grip("chat").focus()
  fireEvent.keyDown(grip("chat"), { key: "ArrowRight" }) // into Tasks' column, whose old column goes
  expect(shown()).toBe("x(y(agents usage) y(chat tasks))")
  expect(document.activeElement).toBe(grip("chat"))
  fireEvent.keyDown(grip("chat"), { key: "ArrowDown" })
  expect(shown()).toBe("x(y(agents usage) y(tasks chat))")
  fireEvent.keyDown(grip("chat"), { key: "ArrowDown" }) // out: along the full width
  expect(shown()).toBe("y(x(y(agents usage) tasks) chat)")
  expect(document.activeElement).toBe(grip("chat"))
  fireEvent.keyDown(grip("chat"), { key: "ArrowDown" })
  expect(onChange).toHaveBeenCalledTimes(3)
})

test("a shut Usage leaves its button where it was, in any stack, and the button reopens it", () => {
  render(<Live onChange={vi.fn()} first={{ axis: "x", sizes: [0.3, 0.4, 0.3], children: ["agents", { axis: "y", sizes: [0, 1], children: ["usage", "chat"] }, "tasks"] }} />)
  expect(shown()).toBe("x(agents y(chat) tasks)")
  const button = screen.getByRole("button", { name: "Show usage" })
  expect(button.nextElementSibling).toBe(slot("chat")) // above Chat, where Usage was
  fireEvent.click(button)
  expect(shown()).toBe("x(agents y(usage chat) tasks)")
  expect(sep("Usage over Chat")).toBeTruthy()
})

test("Usage beside other panes cannot be shut", () => {
  render(<Live onChange={vi.fn()} first={{ axis: "x", sizes: [0.5, 0.5], children: ["usage", { axis: "y", sizes: [0.3, 0.4, 0.3], children: ["agents", "chat", "tasks"] }] }} />)
  expect(screen.queryByRole("button", { name: "Show usage" })).toBeNull()
  expect(screen.getAllByRole("separator")).toHaveLength(3) // one between the columns, two in the stack
})

test("minimum widths follow the panes in a column", () => {
  render(<Live onChange={vi.fn()} first={{ axis: "x", sizes: [0.3, 0.4, 0.3], children: [{ axis: "y", sizes: [0.5, 0.5], children: ["usage", "tasks"] }, "chat", "agents"] }} />)
  const min = (id: PaneId) => (slot(id).closest('[style*="min-width: "]') as HTMLElement).style.minWidth
  expect([min("usage"), min("chat"), min("agents")]).toEqual(["280px", "220px", "220px"])
})

// A custom pane in the layout: it moves, docks and is removed like the built-in ones.
let extMounts = 0
function Notes() {
  const [text, setText] = useState("")
  useEffect(() => { extMounts++ }, [])
  return <Panel title="notes"><textarea aria-label="notes draft" value={text} onChange={e => setText(e.target.value)} /></Panel>
}
function LiveExt({ first, onRemove }: { first: Layout; onRemove: (id: PaneId) => void }) {
  const [saved, setSaved] = useState(first)
  return <HomeLayout saved={saved} onChange={setSaved} onRemove={onRemove} tab="agents" onTab={() => {}} panes={{ ...titled(), "ext:notes": <Notes /> }} />
}
const withNotes: Layout = { axis: "x", sizes: [0.22, 0.43, 0.35], children: [{ axis: "y", sizes: [0.65, 0.35], children: ["agents", "usage"] }, "chat", "ext:notes"] }

test("a custom pane has its own header with a remove button, a grip and a maximize link", () => {
  const onRemove = vi.fn()
  render(<LiveExt first={withNotes} onRemove={onRemove} />)
  expect(screen.getByRole("link", { name: "Maximize ext:notes pane" }).getAttribute("href")).toBe("#/pane/ext:notes")
  fireEvent.click(screen.getByRole("button", { name: "Remove ext:notes pane" }))
  expect(onRemove).toHaveBeenCalledWith("ext:notes")
  expect(screen.getAllByRole("button", { name: /^Remove / })).toHaveLength(5 - 1) // every pane, tasks is not in this layout
})

test("the last pane has no remove button, and without onRemove none has", () => {
  const { unmount } = render(<HomeLayout saved="chat" onChange={vi.fn()} onRemove={vi.fn()} tab="agents" onTab={() => {}} panes={titled()} />)
  expect(screen.queryByRole("button", { name: /^Remove / })).toBeNull()
  unmount()
  view()
  expect(screen.queryByRole("button", { name: /^Remove / })).toBeNull()
})

test("moving a custom pane keeps its component state", () => {
  extMounts = 0
  render(<LiveExt first={withNotes} onRemove={vi.fn()} />)
  fireEvent.change(screen.getByLabelText("notes draft"), { target: { value: "keep me" } })
  grip("ext:notes").focus()
  fireEvent.keyDown(grip("ext:notes"), { key: "ArrowLeft" })
  expect(shown()).not.toBe("x(y(agents usage) chat ext:notes)")
  expect((screen.getByLabelText("notes draft") as HTMLTextAreaElement).value).toBe("keep me")
  expect(extMounts).toBe(1)
})

test("a layout that names a pane with no content still renders the rest", () => {
  render(<HomeLayout saved={{ axis: "x", sizes: [0.5, 0.5], children: ["chat", "ext:gone"] }} onChange={vi.fn()} tab="agents" onTab={() => {}} panes={titled()} />)
  expect(screen.getByLabelText("draft")).toBeTruthy()
})

test("a phone has a tab for each custom pane, and a tab for a deleted pane falls back to Agents", () => {
  window.matchMedia = ((q: string) => ({ matches: false, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
  const onTab = vi.fn()
  const all = { ...panes(), "ext:notes": <p>Notes pane</p> }
  const { rerender } = render(<HomeLayout onChange={vi.fn()} tab="ext:notes" onTab={onTab} panes={all} />)
  expect(screen.getByText("Notes pane")).toBeTruthy()
  expect(screen.getByRole("tab", { name: "notes", selected: true })).toBeTruthy()
  fireEvent.click(screen.getByRole("tab", { name: "Chat" }))
  expect(onTab).toHaveBeenCalledWith("chat")
  rerender(<HomeLayout onChange={vi.fn()} tab="ext:notes" onTab={onTab} panes={panes()} />)
  expect(screen.getByText("Agents pane")).toBeTruthy()
  expect(screen.getByRole("tab", { name: "Agents", selected: true })).toBeTruthy()
})
