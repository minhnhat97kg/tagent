import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react"
import { TasksPane } from "./TasksPane"
import { TaskDrawer } from "@/components/Board"
import type { Project } from "@/types"

// The real board and drawer run here.
vi.mock("@/api", () => ({ api: vi.fn().mockResolvedValue(undefined), post: vi.fn(), act: (p: Promise<unknown>) => p.catch(() => {}) }))

let width = 599
let resized: (() => void)[] = []
const proj = { statuses: [{ id: "todo", role: "", run: "" }], tasks: [{ id: "1", title: "Alpha", text: "Alpha", status: "todo", assignees: null, created: "", updated: "", comments: [] }], roles: [], workflows: [], dir: "/p" } as unknown as Project

beforeEach(() => {
  resized = []
  globalThis.ResizeObserver = class {
    constructor(cb: () => void) { resized.push(cb) }
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver
  window.matchMedia = ((q: string) => ({ matches: true, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, json: async () => [] })))
  vi.spyOn(Element.prototype, "clientWidth", "get").mockImplementation(() => width)
})
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

const at = (w: number) => { width = w; act(() => resized.forEach(f => f())) }
const board = () => !!document.querySelector("section.task-board")
const view = () => render(<><TasksPane proj={proj} dir="/p" reload={() => {}} /><div data-pane="chat"><textarea aria-label="Agent terminal" /></div></>)
const openFromList = () => fireEvent.click(screen.getByRole("button", { name: "#1 todo: Alpha" }))

test("the list below 600px, the board from 600, and back", () => {
  width = 599
  view()
  expect(screen.getByRole("heading", { name: "Tasks" })).toBeTruthy()
  expect(board()).toBe(false)
  at(600)
  expect(board()).toBe(true)
  expect(screen.queryByRole("heading", { name: "Tasks" })).toBeNull()
  at(599)
  expect(board()).toBe(false)
})

test("clicking a ticket in the list opens the drawer, which stays open from list to board and back", () => {
  width = 500
  view()
  openFromList()
  expect(screen.getByRole("dialog")).toBeTruthy()
  expect(document.querySelector("[data-ticket-detail]")).toBeNull()
  at(600)
  expect(board()).toBe(true)
  expect(screen.getAllByRole("dialog")).toHaveLength(1)
  at(500)
  expect(screen.getAllByRole("dialog")).toHaveLength(1)
})

test("a board card opens the same drawer", () => {
  width = 600
  view()
  fireEvent.click(screen.getByRole("button", { name: "#1: Alpha" }))
  expect(screen.getByRole("dialog")).toBeTruthy()
})

test("Escape in the chat pane leaves the drawer open; Escape elsewhere closes it", () => {
  width = 500
  view()
  openFromList()
  fireEvent.keyDown(screen.getByLabelText("Agent terminal"), { key: "Escape" })
  expect(screen.getByRole("dialog")).toBeTruthy()
  fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" })
  expect(screen.queryByRole("dialog")).toBeNull()
})

test("opening a ticket in another drawer closes the pane's, so two never stack", () => {
  width = 500
  const other = (id?: string) => <>
    <TasksPane proj={proj} dir="/p" reload={() => {}} />
    <TaskDrawer proj={proj} dir="/p" reload={() => {}} id={id} onClose={() => {}} origin={{ current: null }} />
  </>
  const { rerender } = render(other())
  openFromList()
  expect(screen.getAllByRole("dialog")).toHaveLength(1)
  rerender(other("1"))
  expect(screen.getAllByRole("dialog")).toHaveLength(1)
})
