import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import App from "./App"

vi.mock("@/api", () => ({ api: vi.fn().mockResolvedValue(undefined), act: vi.fn(), post: vi.fn() }))
vi.mock("@/state", () => ({
  useAppState: () => ({ state: { host: "box", projects: ["/p"], theme: "light", agents: [], statuses: {} }, error: undefined }),
}))
vi.mock("@/components/Board", () => ({ Board: () => <div data-testid="board" />, TaskDrawer: () => null }))
vi.mock("@/components/AgentView", () => ({ AgentView: () => null }))

// Some modules (the terminal) read matchMedia at import time; jsdom has none.
vi.hoisted(() => {
  const fake = (q: string) =>
    ({ matches: false, media: q, onchange: null, addEventListener: () => {}, removeEventListener: () => {}, addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false })
  window.matchMedia = fake as unknown as typeof window.matchMedia
})

// The phone home stacks the same panels, so the rail's copy is the count beyond one.
const usageHeadings = () => screen.queryAllByRole("heading", { name: "Usage" }).length

beforeEach(() => {
  localStorage.clear()
  Element.prototype.scrollTo = vi.fn()
})
afterEach(cleanup)

test("the rail collapses to an icon strip and remembers it", () => {
  render(<App />)
  expect(usageHeadings()).toBe(2)
  expect(screen.queryByLabelText("Panels")).toBeNull()

  fireEvent.click(screen.getByRole("button", { name: "Hide sidebar" }))
  expect(usageHeadings()).toBe(1)
  expect(screen.getByLabelText("Panels")).toBeTruthy()
  expect(localStorage.getItem("tagent.rail")).toBe("collapsed")

  fireEvent.click(screen.getByRole("button", { name: "Show usage panel" }))
  expect(usageHeadings()).toBe(2)
  expect(localStorage.getItem("tagent.rail")).toBe("open")
})

test("a remembered collapsed rail starts collapsed", () => {
  localStorage.setItem("tagent.rail", "collapsed")
  render(<App />)
  expect(usageHeadings()).toBe(1)
  expect(screen.getByRole("button", { name: "Show agents panel" })).toBeTruthy()
})
