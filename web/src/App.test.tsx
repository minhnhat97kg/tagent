import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import App from "./App"
import { api } from "@/api"
import { toast } from "sonner"
import { savePanes } from "@/lib/paneLayout"

vi.mock("sonner", async o => ({ ...(await o<object>()), toast: vi.fn() }))
vi.mock("@/api", () => ({ api: vi.fn().mockResolvedValue(undefined), act: vi.fn(), post: vi.fn() }))
const st = vi.hoisted(() => ({ state: { host: "box", projects: ["/p"], agents: [] as Record<string, unknown>[], statuses: {} } }))
vi.mock("@/state", () => ({ useAppState: () => ({ state: st.state, error: undefined }) }))
vi.mock("@/components/Board", () => ({ Board: () => <div data-testid="board" />, TaskDrawer: () => null, TaskDetail: () => null }))
vi.mock("@/components/AgentView", () => ({ AgentView: ({ agent }: { agent: { pane: string } }) => <div data-testid="agent-view">{agent.pane}</div> }))

// Some modules (the terminal) read matchMedia at import time; jsdom has none. Tests default to a phone width.
const phoneMedia = vi.hoisted(() => {
  const fake = (q: string) =>
    ({ matches: false, media: q, onchange: null, addEventListener: () => {}, removeEventListener: () => {}, addListener: () => {}, removeListener: () => {}, dispatchEvent: () => false })
  window.matchMedia = fake as unknown as typeof window.matchMedia
  return fake
})
// The home split observes its size; jsdom has no ResizeObserver.
globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver

const desktop = () => {
  window.matchMedia = ((q: string) => ({ matches: q.includes("min-width: 768px"), media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
}
const pane = (name: string) => screen.getByRole("separator", { name })
// Reset layout lives on the Settings "This device" tab and returns home.
const resetLayout = async () => {
  fireEvent.click(screen.getByRole("button", { name: "Settings" }))
  fireEvent.click(await screen.findByRole("link", { name: "This device" }))
  fireEvent.click(await screen.findByRole("button", { name: "Reset layout" }))
  await waitFor(() => expect(document.querySelector("[data-pane]")).toBeTruthy())
}
const agent = (pane: string, name: string) => ({ pane, name, path: "/p", agent: "claude", status: "idle", project: "p", prompt: "" })

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
  vi.restoreAllMocks()
  window.matchMedia = phoneMedia as unknown as typeof window.matchMedia
})

test("a phone home shows the agents tab: usage over the agents; Tasks is its own tab", () => {
  render(<App />)
  expect(screen.getAllByRole("heading", { name: "Usage" })).toHaveLength(1)
  expect(screen.getByRole("heading", { name: "Agents" })).toBeTruthy()
  expect(screen.queryByRole("heading", { name: "Tasks" })).toBeNull()
  expect(screen.getByRole("tab", { name: "Tasks" }).closest("header")).toBeTruthy() // the tabs sit in the app header
  fireEvent.click(screen.getByRole("tab", { name: "Tasks" }))
  expect(screen.getByRole("heading", { name: "Tasks" })).toBeTruthy()
})

test("desktop home has the agents, usage, chat and tasks panes, and the header has no Agents or Workflows buttons", () => {
  desktop()
  render(<App />)
  expect(Array.from(document.querySelectorAll("[data-pane]"), p => p.getAttribute("data-pane"))).toEqual(["agents", "usage", "chat"])
  expect(screen.getByRole("tablist", { name: "Tasks or workflows" })).toBeTruthy()
  expect(screen.getByText("Pick an agent")).toBeTruthy()
  expect(screen.queryByRole("button", { name: "Agents" })).toBeNull()
  expect(screen.queryByRole("button", { name: "Workflows" })).toBeNull()
})

test("an old link opens its page maximized, and a new #/pane link does too", () => {
  for (const [hash, pane] of [["#/pane/usage", "usage"], ["#/pane/tasks", "tasks"], ["#/pane/chat", "chat"], ["#/pane/agents", "agents"], ["#/agents", "agents"], ["#/pane/workflows", "workflows"], ["#/workflows", "workflows"], ["#/board/config", "workflows"]] as const) {
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

test("every settings page sits under the same tabs, and a tab asks before dropping a draft", () => {
  for (const [hash, tab] of [["#/settings", "General"], ["#/agents", "Agents"], ["#/routing", "Routing"], ["#/workflows", "Workflows"], ["#/board/config", "Board"], ["#/settings/workspaces", "Workspaces"], ["#/settings/device", "This device"]]) {
    cleanup()
    location.hash = hash
    render(<App />)
    expect(screen.getByRole("navigation", { name: "Settings" }).querySelector('[aria-current="page"]')?.textContent, hash).toBe(tab)
  }
})

test("the rail and the Board page are gone; #/board lands on home", () => {
  desktop()
  location.hash = "#/board"
  render(<App />)
  expect(location.hash).toBe("#/")
  expect(screen.queryByRole("button", { name: "Board" })).toBeNull()
  expect(screen.queryByRole("button", { name: "Hide sidebar" })).toBeNull()
  expect(screen.getByRole("heading", { name: "Tasks" })).toBeTruthy()
})

test("a saved layout is applied after a remount, and Reset layout restores the default", async () => {
  desktop()
  localStorage.setItem("tagent.panes", JSON.stringify({ v: 2, cols: [0.3, 0.4, 0.3], left: [0.5, 0.5] }))
  const { unmount } = render(<App />)
  expect(pane("Agents and Chat").getAttribute("aria-valuenow")).toBe("30")
  unmount()

  render(<App />)
  expect(pane("Agents and Chat").getAttribute("aria-valuenow")).toBe("30")
  await resetLayout()
  expect(localStorage.getItem("tagent.panes")).toBeNull()
  expect(pane("Agents and Chat").getAttribute("aria-valuenow")).toBe("22")
})

test("desktop panes have a move grip; a saved order applies and Reset layout restores the default", async () => {
  desktop()
  localStorage.setItem("tagent.panes", JSON.stringify({ v: 2, cols: [0.3, 0.4, 0.3], left: [0.5, 0.5], order: ["tasks", "chat", "usage", "agents"] }))
  render(<App />)
  const placed = () => Array.from(document.querySelectorAll("[data-pane-slot]"), s => s.querySelector('button[aria-label^="Move "]')?.getAttribute("aria-label")?.split(" ")[1])
  expect(placed()).toEqual(["tasks", "chat", "usage", "agents"])
  await resetLayout()
  expect(placed()).toEqual(["agents", "usage", "chat", "tasks"])
})

test("a phone and a maximized pane have no move grip", () => {
  render(<App />)
  expect(document.querySelector('button[aria-label^="Move "]')).toBeNull()
  cleanup()
  desktop()
  location.hash = "#/pane/usage"
  render(<App />)
  expect(document.querySelector('button[aria-label^="Move "]')).toBeNull()
})

test("desktop pane headers maximize, and the maximized page minimizes back from the same spot", () => {
  desktop()
  render(<App />)
  for (const id of ["agents", "usage", "chat", "tasks"]) expect(screen.getByRole("link", { name: `Maximize ${id} pane` }).getAttribute("href")).toBe(`#/pane/${id}`)
  fireEvent.click(screen.getByRole("tab", { name: "workflows" }))
  expect(screen.getByRole("link", { name: "Maximize workflows pane" }).getAttribute("href")).toBe("#/pane/workflows")
  for (const id of ["agents", "usage", "chat", "tasks", "workflows"]) {
    cleanup()
    location.hash = `#/pane/${id}`
    render(<App />)
    expect(screen.getByRole("link", { name: `Minimize ${id} pane` }).getAttribute("href"), id).toBe("#/")
    expect(screen.getAllByRole("link", { name: /^M(ax|in)imize/ }), id).toHaveLength(1)
  }
})

test("the saved grid layout (v:1) is ignored, and the default layout shows", () => {
  desktop()
  localStorage.setItem("tagent.panes", JSON.stringify({ v: 1, boxes: [{ id: "usage", x: 0, y: 5, w: 3, h: 10 }] }))
  render(<App />)
  expect(pane("Agents and Chat").getAttribute("aria-valuenow")).toBe("22")
})

test("desktop: clicking an agent shows it in the chat pane, and the route does not change", () => {
  desktop()
  st.state.agents = [agent("%1", "alpha")]
  render(<App />)
  fireEvent.click(screen.getByRole("button", { name: /alpha/ }))
  expect(screen.getByTestId("agent-view").textContent).toBe("%1")
  expect(screen.getByTestId("agent-view").closest('[data-pane="chat"]')).toBeTruthy()
  expect(location.hash).toBe("")
})

test("an old agent link opens the one-page layout with that agent in the chat pane", () => {
  desktop()
  st.state.agents = [agent("%1", "alpha")]
  for (const hash of ["#/agent/%251", "#/agents/%251"]) {
    cleanup()
    location.hash = hash
    render(<App />)
    expect(document.querySelector("[data-max]"), hash).toBeNull()
    expect(screen.getByTestId("agent-view").textContent).toBe("%1")
    expect(pane("Agents and Chat")).toBeTruthy()
  }
})

test("phone: selecting an agent switches to the Chat tab", () => {
  st.state.agents = [agent("%1", "alpha")]
  render(<App />)
  fireEvent.click(screen.getByRole("button", { name: /alpha/ }))
  expect(screen.getByRole("tab", { name: "Chat" }).getAttribute("aria-selected")).toBe("true")
  expect(screen.getByTestId("agent-view").textContent).toBe("%1")
})

test("a wide Tasks column renders the board; its Workflows tab replaces it", () => {
  desktop()
  vi.spyOn(Element.prototype, "clientWidth", "get").mockReturnValue(1400)
  render(<App />)
  expect(screen.getByTestId("board")).toBeTruthy()
  fireEvent.click(screen.getByRole("tab", { name: "workflows" }))
  expect(screen.queryByTestId("board")).toBeNull()
  expect(screen.getByRole("tab", { name: "workflows" }).getAttribute("aria-selected")).toBe("true")
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

test("the header shows Custom only when a custom pane exists", async () => {
  desktop()
  vi.mocked(api).mockImplementation(async p => (p === "/api/ext" ? [{ name: "hello", agent: false, actions: false }] : undefined))
  render(<App />)
  expect(await screen.findByRole("link", { name: "Custom" })).toBeTruthy()
})

test("no custom panes, no Custom link", async () => {
  desktop()
  render(<App />)
  await waitFor(() => expect(vi.mocked(api)).toHaveBeenCalledWith("/api/ext"))
  expect(screen.queryByRole("link", { name: "Custom" })).toBeNull()
})

const exts = (...e: { name: string; agent?: boolean; actions?: boolean }[]) => {
  const list = e.map(x => ({ agent: false, actions: false, ...x }))
  vi.mocked(api).mockImplementation(async p => (p === "/api/ext" ? list : undefined))
  return list
}
test("a custom pane in the saved layout is a home pane with its header; the Add pane menu offers one that is not", async () => {
  desktop()
  exts({ name: "notes" }, { name: "bot", agent: true })
  savePanes({ axis: "x", sizes: [0.5, 0.5], children: ["chat", "ext:notes"] })
  render(<App />)
  expect(await screen.findByTitle("notes")).toBeTruthy()
  expect(screen.queryByTitle("bot")).toBeNull()
  expect(screen.getByRole("heading", { name: "notes" })).toBeTruthy()
  fireEvent.keyDown(screen.getByRole("button", { name: "Add pane" }), { key: "Enter" })
  expect(await screen.findByRole("menuitem", { name: /^bot/ })).toBeTruthy()
  expect(screen.getByRole("menuitem", { name: "Agents" })).toBeTruthy()
  expect(screen.queryByRole("menuitem", { name: "notes" })).toBeNull()
  fireEvent.click(screen.getByRole("menuitem", { name: /^bot/ }))
  expect(await screen.findByTitle("bot")).toBeTruthy()
  expect(JSON.stringify(JSON.parse(localStorage.getItem("tagent.panes")!).root)).toContain("ext:bot")
})

test("a saved layout that names a deleted pane still loads, without it", async () => {
  desktop()
  exts({ name: "notes" })
  savePanes({ axis: "x", sizes: [0.4, 0.3, 0.3], children: ["chat", "ext:gone", "ext:notes"] })
  render(<App />)
  expect(await screen.findByTitle("notes")).toBeTruthy()
  expect(screen.queryByTitle("gone")).toBeNull()
  expect(screen.getByText("Pick an agent")).toBeTruthy()
})

test("removing a pane from its header takes it out of the layout and the menu offers it again", async () => {
  desktop()
  exts({ name: "notes" })
  savePanes({ axis: "x", sizes: [0.5, 0.5], children: ["chat", "ext:notes"] })
  render(<App />)
  await screen.findByTitle("notes")
  fireEvent.click(screen.getByRole("button", { name: "Remove ext:notes pane" }))
  expect(screen.queryByTitle("notes")).toBeNull()
  expect(localStorage.getItem("tagent.panes")).not.toContain("ext:notes")
  fireEvent.keyDown(screen.getByRole("button", { name: "Add pane" }), { key: "Enter" })
  expect(await screen.findByRole("menuitem", { name: "notes" })).toBeTruthy()
})

test("an agent-made pane shows its badge and a switch for actions", async () => {
  desktop()
  exts({ name: "bot", agent: true }, { name: "bot2", agent: true, actions: true })
  savePanes({ axis: "x", sizes: [0.4, 0.3, 0.3], children: ["chat", "ext:bot", "ext:bot2"] })
  render(<App />)
  await screen.findByTitle("bot")
  expect(screen.getByText("agent · read-only")).toBeTruthy()
  expect(screen.getByText("agent · actions allowed")).toBeTruthy()
  expect(screen.getByRole("button", { name: "Allow actions" })).toBeTruthy()
  expect(screen.getByRole("button", { name: "Revoke" })).toBeTruthy()
})

test("a new agent-made pane while the app is open is offered in a toast, not added", async () => {
  desktop()
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    exts({ name: "old", agent: true })
    render(<App />)
    await waitFor(() => expect(vi.mocked(api)).toHaveBeenCalledWith("/api/ext"))
    await vi.advanceTimersByTimeAsync(10)
    expect(toast).not.toHaveBeenCalled() // the first list is the baseline
    exts({ name: "old", agent: true }, { name: "fresh", agent: true }, { name: "mine" })
    await vi.advanceTimersByTimeAsync(5100)
    await waitFor(() => expect(toast).toHaveBeenCalledTimes(1))
    expect(vi.mocked(toast).mock.calls[0]![0]).toBe("Agent added pane fresh")
    expect(screen.queryByTitle("fresh")).toBeNull()
    expect(localStorage.getItem("tagent.panes")).toBeNull()
    const action = (vi.mocked(toast).mock.calls[0]![1] as { action: { label: string; onClick: () => void } }).action
    expect(action.label).toBe("Add")
    action.onClick()
    expect(await screen.findByTitle("fresh")).toBeTruthy()
  } finally {
    vi.useRealTimers()
  }
})

test("#/pane/ext:<name> opens a custom pane maximized", async () => {
  exts({ name: "notes" })
  location.hash = "#/pane/ext:notes"
  render(<App />)
  expect(await screen.findByTitle("notes")).toBeTruthy()
  expect(document.querySelector('[data-max="ext:notes"]')).toBeTruthy()
  expect(screen.getByRole("link", { name: "Minimize ext:notes pane" }).getAttribute("href")).toBe("#/")
})
