import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react"
import { TasksPanel } from "./TasksPanel"
import { useKeys } from "@/hooks/useKeys"
import type { Agent, Project } from "@/types"

// The real ticket panel (BoardDrawer) runs here: the inline tier must not need a Radix dialog around it.
const mocks = vi.hoisted(() => ({ api: vi.fn() }))
vi.mock("@/api", () => ({ api: mocks.api, post: vi.fn(), act: (p: Promise<unknown>) => p.catch(() => {}) }))

const proj = {
  statuses: [{ id: "todo", role: "", run: "" }, { id: "build", role: "builder", run: "" }],
  tasks: [{ id: "1", title: "Alpha", text: "Alpha\nThe ticket's text", status: "todo", assignees: null, created: "2026-11-23T09:00:00Z", updated: "2026-11-23T10:00:00Z", comments: [] }],
  roles: ["builder"],
  workflows: [],
  dir: "/p",
} as unknown as Project
const agent = { pane: "%1", name: "a", agent: "claude", status: "idle", session: "", window: "", path: "/p", project: "p", prompt: "" } as Agent

// Keys are bound the way the home view binds them, so a j press would walk to the agent.
function KeysProbe() {
  useKeys([agent], undefined)
  return null
}

beforeEach(() => {
  mocks.api.mockReset().mockResolvedValue(undefined)
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, json: async () => [] })))
  globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver
  window.matchMedia = ((q: string) => ({ matches: true, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
  location.hash = "#/"
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

test("split: the ticket opens beside the list with no dialog, and j inside it does not walk the agents", () => {
  render(<><TasksPanel proj={proj} dir="/p" reload={() => {}} split /><KeysProbe /></>)
  expect(document.querySelector("[data-ticket-detail]")?.textContent).toContain("Select a task")

  fireEvent.click(screen.getByRole("button", { name: "#1 todo: Alpha" }))
  const detail = document.querySelector<HTMLElement>("[data-ticket-detail]")!
  expect(within(detail).getByRole("tablist", { name: "Ticket panel" })).toBeTruthy()
  expect(screen.queryByRole("dialog")).toBeNull()
  expect(screen.getByRole("button", { name: "#1 todo: Alpha" }).getAttribute("aria-current")).toBe("true")

  fireEvent.keyDown(within(detail).getByRole("tab", { name: "Details" }), { key: "j" })
  expect(location.hash).toBe("#/")
  fireEvent.keyDown(document.body, { key: "j" })
  expect(location.hash).toBe("#/agent/%251")
})

test("list: the ticket opens in the drawer, not beside the list", () => {
  render(<TasksPanel proj={proj} dir="/p" reload={() => {}} />)
  expect(document.querySelector("[data-ticket-detail]")).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: "#1 todo: Alpha" }))
  expect(screen.getByRole("dialog")).toBeTruthy()
})
