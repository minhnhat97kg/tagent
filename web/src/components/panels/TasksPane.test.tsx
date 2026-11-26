import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react"
import { TasksPane } from "./TasksPane"
import type { Project } from "@/types"

vi.mock("@/components/Board", () => ({ Board: () => <div data-testid="board" />, TaskDrawer: () => null, TaskDetail: ({ id }: { id?: string }) => (id ? <div data-testid="detail" /> : null) }))

let width = 899
let resized = () => {}
let desktop = true
const proj = { statuses: [{ id: "todo", role: "", run: "" }], tasks: [{ id: "1", title: "Alpha", text: "Alpha", status: "todo", assignees: null, created: "", updated: "", comments: null }], roles: [], dir: "/p" } as unknown as Project

beforeEach(() => {
  globalThis.ResizeObserver = class {
    constructor(cb: () => void) { resized = cb }
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver
  window.matchMedia = ((q: string) => ({ matches: desktop, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
  vi.spyOn(Element.prototype, "clientWidth", "get").mockImplementation(() => width)
  desktop = true
})
afterEach(() => { cleanup(); vi.restoreAllMocks() })

const at = (w: number) => { width = w; act(() => resized()) }
const tier = () => (screen.queryByTestId("board") ? "board" : document.querySelector("[data-ticket-detail]") ? "split" : "list")

test("the list below 480px, the list with the ticket beside it from 480, the board from 900, and back", () => {
  width = 479
  render(<TasksPane proj={proj} dir="/p" reload={() => {}} />)
  expect(screen.getByRole("heading", { name: "Tasks" })).toBeTruthy()
  expect(tier()).toBe("list")
  at(480)
  expect(tier()).toBe("split")
  expect(screen.getByRole("heading", { name: "Tasks" })).toBeTruthy()
  at(899)
  expect(tier()).toBe("split")
  at(900)
  expect(tier()).toBe("board")
  expect(screen.queryByRole("heading", { name: "Tasks" })).toBeNull()
  at(899)
  expect(tier()).toBe("split")
  at(479)
  expect(tier()).toBe("list")
})

test("a ticket opened in the split tier stays open when the pane goes back to the list", () => {
  width = 600
  render(<TasksPane proj={proj} dir="/p" reload={() => {}} />)
  fireEvent.click(screen.getByRole("button", { name: "#1 todo: Alpha" }))
  expect(screen.getByTestId("detail")).toBeTruthy()
  at(479)
  expect(tier()).toBe("list")
  at(600)
  expect(screen.getByTestId("detail")).toBeTruthy()
})

test("a phone keeps the list at any width below 900px", () => {
  desktop = false
  width = 600
  render(<TasksPane proj={proj} dir="/p" reload={() => {}} />)
  expect(tier()).toBe("list")
})
