import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { HomeLayout } from "./HomeLayout"
import { moveDivider } from "./SplitPane"
import { DEFAULT_PANES, type PaneSizes } from "@/lib/paneLayout"

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

const view = (onChange = vi.fn(), saved?: PaneSizes) => {
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
  const { cols, left } = onChange.mock.calls[0]![0] as PaneSizes
  expect(cols[0]! * 1000).toBeCloseTo(0.22 * 1000 + 100)
  expect(sum(cols)).toBeCloseTo(1)
  expect(left).toEqual(DEFAULT_PANES.left)
})

test("a drag cannot squeeze Agents below its minimum, and a cancelled drag saves nothing", () => {
  const onChange = view(vi.fn(), { cols: [0.3, 0.4, 0.3], left: [0.65, 0.35] })
  const divider = sep("Agents and Chat")
  fireEvent(divider, ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", -900, 0))
  fireEvent(window, ptr("pointerup", -900, 0))
  const { cols } = onChange.mock.calls[0]![0] as PaneSizes
  expect(cols[0]! * 1000).toBeCloseTo(220)

  fireEvent(divider, ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", 200, 0))
  fireEvent(window, ptr("pointercancel", 200, 0))
  expect(onChange).toHaveBeenCalledTimes(1)
})

test("arrow keys on a focused divider move it by 2% and save at once; double-click resets", () => {
  const onChange = view(vi.fn(), { cols: [0.3, 0.4, 0.3], left: [0.5, 0.5] })
  const divider = sep("Agents and Chat")
  expect(divider.getAttribute("aria-valuenow")).toBe("30")
  expect(divider.getAttribute("aria-orientation")).toBe("vertical")
  fireEvent.keyDown(divider, { key: "ArrowRight" })
  expect((onChange.mock.calls[0]![0] as PaneSizes).cols[0]).toBeCloseTo(0.32)
  fireEvent.keyDown(divider, { key: "ArrowDown" }) // the other axis does nothing here
  expect(onChange).toHaveBeenCalledTimes(1)
  fireEvent.doubleClick(divider)
  expect(onChange.mock.calls[1]![0]).toEqual({ cols: DEFAULT_PANES.cols, left: [0.5, 0.5] })
})

test("the Agents over Usage divider is horizontal; its arrow keys move it, and dragging it down collapses Usage", () => {
  const onChange = view()
  const divider = sep("Agents over Usage")
  expect(divider.getAttribute("aria-orientation")).toBe("horizontal")
  fireEvent.keyDown(divider, { key: "ArrowUp" })
  expect((onChange.mock.calls[0]![0] as PaneSizes).left[1]).toBeCloseTo(0.37)

  fireEvent(divider, ptr("pointerdown", 0, 0))
  fireEvent(window, ptr("pointermove", 0, 600))
  fireEvent(window, ptr("pointerup", 0, 600))
  expect((onChange.mock.calls[1]![0] as PaneSizes).left).toEqual([1, 0])
})

test("a collapsed Usage shows a button that reopens it at the default size", () => {
  const onChange = view(vi.fn(), { cols: DEFAULT_PANES.cols, left: [1, 0] })
  expect(screen.queryByText("Usage pane")).toBeNull()
  expect(screen.queryByRole("separator", { name: "Agents over Usage" })).toBeNull() // no divider to a shut pane
  fireEvent.click(screen.getByRole("button", { name: "Show usage" }))
  expect(onChange).toHaveBeenCalledWith({ cols: DEFAULT_PANES.cols, left: DEFAULT_PANES.left })
})

test("a shut pane opens at 64px on a small step, and an open one shuts below 64px", () => {
  expect(moveDivider([0.65, 0], 0, -2, 800, [128, 0], 1)[1]! * 800).toBeCloseTo(64)
  expect(moveDivider([0.65, 0.35], 0, 0, 800, [128, 0], 1)).toEqual([0.65, 0.35])
  expect(moveDivider([0.92, 0.08], 0, 10, 800, [128, 0], 1)[1]).toBe(0)
})

test("each desktop pane links to its maximized page", () => {
  view()
  expect(screen.getByRole("link", { name: "Maximize agents pane" }).getAttribute("href")).toBe("#/pane/agents")
  expect(screen.getByRole("link", { name: "Maximize chat pane" }).getAttribute("href")).toBe("#/pane/chat")
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
