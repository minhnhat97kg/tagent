import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import App from "./App"
import { api } from "@/api"

vi.mock("@/api", () => ({ api: vi.fn().mockResolvedValue(undefined), act: vi.fn(), post: vi.fn() }))
const st = vi.hoisted(() => ({ state: { host: "box", projects: ["/p"], agents: [] as Record<string, unknown>[], statuses: {} } }))
vi.mock("@/state", () => ({ useAppState: () => ({ state: st.state, error: undefined }) }))
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
  location.hash = ""
  st.state.projects = ["/p"]
  st.state.agents = []
  vi.mocked(api).mockReset().mockResolvedValue(undefined)
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

test("opening an agent makes its project the remembered one", () => {
  st.state.projects = ["/p/a", "/p/b"]
  st.state.agents = [{ pane: "%1", path: "/p/b/wt", agent: "claude", status: "idle", project: "b", prompt: "" }]
  location.hash = "#/agent/%251"
  render(<App />)
  cleanup()
  location.hash = "#/"
  render(<App />)
  expect(localStorage.getItem("ta.project")).toBe("/p/b")
  expect(vi.mocked(api)).toHaveBeenCalledWith(expect.stringContaining("/api/project?dir=%2Fp%2Fb"))
})

test("a run link names its project, not the picker's", () => {
  st.state.projects = ["/p/a", "/p/b"]
  localStorage.setItem("ta.project", "/p/b")
  location.hash = "#/runs/61?dir=%2Fp%2Fa"
  render(<App />)
  expect(vi.mocked(api)).toHaveBeenCalledWith(expect.stringContaining("/api/tasks/61/run?dir=%2Fp%2Fa"))
  expect(localStorage.getItem("ta.project")).toBe("/p/a")
})

test("a run link switches the picker once the project list has loaded", () => {
  st.state.projects = ["/p/b"]
  location.hash = "#/runs/61?dir=%2Fp%2Fa"
  const { rerender } = render(<App />)
  expect(vi.mocked(api)).not.toHaveBeenCalledWith(expect.stringContaining("dir=%2Fp%2Fa"))
  st.state.projects = ["/p/a", "/p/b"]
  rerender(<App />)
  expect(vi.mocked(api)).toHaveBeenCalledWith(expect.stringContaining("/api/tasks/61/run?dir=%2Fp%2Fa"))
  expect(localStorage.getItem("ta.project")).toBe("/p/a")
})

test("a run that fails to load names the project and offers the board", async () => {
  st.state.projects = ["/p/a"]
  location.hash = "#/runs/61?dir=%2Fp%2Fa"
  vi.mocked(api).mockImplementation((async (url: string) => {
    if (url.includes("/run")) throw new Error("ticket not found")
    return undefined
  }) as never)
  render(<App />)
  expect((await screen.findByRole("alert")).textContent).toBe("Couldn't load the run in a. ticket not found")
  expect(screen.getByRole("button", { name: "Back to board" })).toBeTruthy()
})
