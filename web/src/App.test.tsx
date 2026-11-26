import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import App from "./App"
import { api } from "@/api"

vi.mock("@/api", () => ({ api: vi.fn().mockResolvedValue(undefined), act: vi.fn(), post: vi.fn() }))
const st = vi.hoisted(() => ({ state: { host: "box", projects: ["/p"], agents: [] as Record<string, unknown>[], statuses: {} } }))
vi.mock("@/state", () => ({ useAppState: () => ({ state: st.state, error: undefined }) }))
vi.mock("@/components/Board", () => ({ Board: () => <div data-testid="board" />, TaskDrawer: () => null, TaskDetail: () => null }))
vi.mock("@/components/AgentView", () => ({ AgentView: () => null }))

// Some modules (the terminal) read matchMedia at import time; jsdom has none. Tests default to a phone width.
const phoneMedia = vi.hoisted(() => {
  const fake = (q: string) =>
    ({ matches: false, media: q, onchange: null, addEventListener: () => {}, removeEventListener: () => {}, addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false })
  window.matchMedia = fake as unknown as typeof window.matchMedia
  return fake
})
// The pane grid observes its size; jsdom has no ResizeObserver.
globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver

const desktop = () => {
  window.matchMedia = ((q: string) => ({ matches: q.includes("min-width: 768px"), media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
}
const pane = (id: string) => document.querySelector<HTMLElement>(`[data-pane="${id}"]`)

beforeEach(() => {
  localStorage.clear()
  location.hash = ""
  st.state.projects = ["/p"]
  st.state.agents = []
  vi.mocked(api).mockReset().mockResolvedValue(undefined)
  Element.prototype.scrollTo = vi.fn()
})
afterEach(() => {
  cleanup()
  window.matchMedia = phoneMedia as unknown as typeof window.matchMedia
})

test("home shows the usage, agents and tasks panes", () => {
  render(<App />)
  expect(screen.getAllByRole("heading", { name: "Usage" })).toHaveLength(1)
  expect(screen.getByRole("heading", { name: "Agents" })).toBeTruthy()
  expect(screen.getByRole("heading", { name: "Tasks" })).toBeTruthy()
})

test("desktop home has four panes, and the header has no Agents or Workflows buttons", () => {
  desktop()
  render(<App />)
  expect(Array.from(document.querySelectorAll("[data-pane]"), p => p.getAttribute("data-pane"))).toEqual(["usage", "agents", "workflows", "tasks"])
  expect(screen.queryByRole("button", { name: "Agents" })).toBeNull()
  expect(screen.queryByRole("button", { name: "Workflows" })).toBeNull()
})

test("an old link opens its pane maximized, and a new #/pane link does too", () => {
  for (const [hash, pane] of [["#/pane/usage", "usage"], ["#/pane/tasks", "tasks"], ["#/pane/agents", "agents"], ["#/agents", "agents"], ["#/agent/%251", "agents"], ["#/pane/workflows", "workflows"], ["#/workflows", "workflows"], ["#/board/config", "workflows"]] as const) {
    cleanup()
    location.hash = hash
    render(<App />)
    expect(document.querySelector(`[data-max="${pane}"]`), hash).toBeTruthy()
    expect(document.querySelector("[data-pane]"), hash).toBeNull()
  }
})

test("the More menu reaches Agents and Workflows at every width below the desktop pane grid", () => {
  render(<App />)
  expect(document.querySelector('button[aria-label="More"]')!.className).toContain("md:hidden")
})

test("#/routing is a page of its own, with no pane", () => {
  location.hash = "#/routing"
  render(<App />)
  expect(document.querySelector("[data-max]")).toBeNull()
})

test("the rail and the Board page are gone; #/board lands on home", () => {
  location.hash = "#/board"
  render(<App />)
  expect(location.hash).toBe("#/")
  expect(screen.queryByRole("button", { name: "Board" })).toBeNull()
  expect(screen.queryByRole("button", { name: "Hide sidebar" })).toBeNull()
  expect(screen.getByRole("heading", { name: "Tasks" })).toBeTruthy()
})

test("a saved layout is applied after a remount, and Reset layout restores the default", () => {
  desktop()
  const saved = [
    { id: "usage", x: 0, y: 5, w: 3, h: 10 },
    { id: "agents", x: 0, y: 15, w: 3, h: 14 },
    { id: "tasks", x: 3, y: 0, w: 9, h: 24 },
  ]
  localStorage.setItem("tagent.panes", JSON.stringify({ v: 1, boxes: saved }))
  const { unmount } = render(<App />)
  expect(pane("usage")!.style.gridRow).toBe("6 / span 10")
  unmount()

  render(<App />)
  expect(pane("usage")!.style.gridRow).toBe("6 / span 10")
  fireEvent.click(screen.getByRole("button", { name: "Settings" }))
  fireEvent.click(screen.getByRole("button", { name: "Reset layout" }))
  expect(localStorage.getItem("tagent.panes")).toBeNull()
  expect(pane("usage")!.style.gridRow).toBe("1 / span 10")
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
