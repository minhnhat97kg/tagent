import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { PaneGrid } from "./PaneGrid"
import type { PaneBox } from "@/lib/paneLayout"

// A pointer event as the grid reads it: clientX/clientY and button.
const ptr = (type: string, x = 0, y = 0) => Object.assign(new Event(type, { bubbles: true, cancelable: true }), { clientX: x, clientY: y, button: 0 })
const panes = () => ({
  usage: <header data-pane-head>Usage</header>,
  agents: <header data-pane-head>Agents</header>,
  workflows: <header data-pane-head>Workflows</header>,
  tasks: <header data-pane-head><button>Add</button></header>,
})
const cell = (id: string) => document.querySelector<HTMLElement>(`[data-pane="${id}"]`)!
const saved = (onChange: ReturnType<typeof vi.fn>) => onChange.mock.calls[0]![0] as PaneBox[]
const box = (boxes: PaneBox[], id: string) => boxes.find(b => b.id === id)

beforeEach(() => {
  // 1200px wide: one unit is 100px; the rows are 24px.
  globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver
  vi.spyOn(Element.prototype, "clientWidth", "get").mockReturnValue(1200)
  window.matchMedia = ((q: string) => ({ matches: true, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
})
afterEach(() => { cleanup(); vi.restoreAllMocks() })

test("the default placement: usage and agents on the left, tasks beside them", () => {
  render(<PaneGrid onChange={vi.fn()} panes={panes()} />)
  expect(cell("usage").style.gridColumn).toBe("1 / span 3")
  expect(cell("usage").style.gridRow).toBe("1 / span 10")
  expect(cell("agents").style.gridRow).toBe("11 / span 6")
  expect(cell("workflows").style.gridColumn).toBe("1 / span 3")
  expect(cell("workflows").style.gridRow).toBe("17 / span 8")
  expect(cell("tasks").style.gridColumn).toBe("4 / span 9")
  expect(cell("tasks").style.gridRow).toBe("1 / span 24")
})

test("each desktop pane has a maximize link to its own route", () => {
  render(<PaneGrid onChange={vi.fn()} panes={panes()} />)
  expect(screen.getByRole("link", { name: "Maximize workflows pane" }).getAttribute("href")).toBe("#/pane/workflows")
  expect(screen.getByRole("link", { name: "Maximize tasks pane" }).getAttribute("href")).toBe("#/pane/tasks")
})

test("dragging a header moves the pane and pushes the one it lands on down", () => {
  const onChange = vi.fn()
  render(<PaneGrid onChange={onChange} panes={panes()} />)
  fireEvent(screen.getByText("Usage"), ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", 300, 0))
  fireEvent(window, ptr("pointerup", 300, 0))
  expect(onChange).toHaveBeenCalledTimes(1)
  expect(box(saved(onChange), "usage")).toMatchObject({ x: 3, y: 0 })
  expect(box(saved(onChange), "tasks")).toMatchObject({ x: 3, y: 10 })
})

test("the corner handle resizes the pane", () => {
  const onChange = vi.fn()
  render(<PaneGrid onChange={onChange} panes={panes()} />)
  const corner = screen.getByRole("button", { name: "Resize usage pane" })
  fireEvent(corner, ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", 300, 0))
  fireEvent(window, ptr("pointerup", 300, 0))
  expect(box(saved(onChange), "usage")).toMatchObject({ x: 0, w: 6 })
})

test("arrow keys on the handles move and resize one step, and save at once", () => {
  const onChange = vi.fn()
  render(<PaneGrid onChange={onChange} panes={panes()} />)
  fireEvent.keyDown(screen.getByRole("button", { name: "Move usage pane" }), { key: "ArrowRight" })
  expect(box(saved(onChange), "usage")).toMatchObject({ x: 1 })
  fireEvent.keyDown(screen.getByRole("button", { name: "Resize usage pane" }), { key: "ArrowDown" })
  expect(box(onChange.mock.calls[1]![0] as PaneBox[], "usage")).toMatchObject({ h: 11 })
})

test("a plain click on a header, or a press on a button in it, saves nothing", () => {
  const onChange = vi.fn()
  render(<PaneGrid onChange={onChange} panes={panes()} />)
  fireEvent(screen.getByText("Usage"), ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointerup", 0, 0))
  fireEvent(screen.getByRole("button", { name: "Add" }), ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", 300, 0))
  fireEvent(window, ptr("pointerup", 300, 0))
  expect(onChange).not.toHaveBeenCalled()
})

test("a cancelled drag discards the move", () => {
  const onChange = vi.fn()
  render(<PaneGrid onChange={onChange} panes={panes()} />)
  fireEvent(screen.getByText("Usage"), ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", 300, 0))
  fireEvent(window, ptr("pointercancel", 300, 0))
  fireEvent(window, ptr("pointerup", 300, 0))
  expect(onChange).not.toHaveBeenCalled()
})

test("a phone stacks the panes with no handles, and a header press does nothing", () => {
  window.matchMedia = ((q: string) => ({ matches: false, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
  const onChange = vi.fn()
  render(<PaneGrid onChange={onChange} panes={panes()} />)
  expect(screen.queryAllByRole("button", { name: /pane$/ })).toHaveLength(0)
  expect(document.querySelector("[data-pane]")).toBeNull()
  expect(screen.queryByText("Workflows")).toBeNull()
  fireEvent(screen.getByText("Usage"), ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", 300, 0))
  fireEvent(window, ptr("pointerup", 300, 0))
  expect(onChange).not.toHaveBeenCalled()
})
