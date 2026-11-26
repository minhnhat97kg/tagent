import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { act, cleanup, render, screen } from "@testing-library/react"
import { TasksPane } from "./TasksPane"

vi.mock("@/components/Board", () => ({ Board: () => <div data-testid="board" />, TaskDrawer: () => null }))

let width = 899
let resized = () => {}

beforeEach(() => {
  globalThis.ResizeObserver = class {
    constructor(cb: () => void) { resized = cb }
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver
  vi.spyOn(Element.prototype, "clientWidth", "get").mockImplementation(() => width)
})
afterEach(() => { cleanup(); vi.restoreAllMocks() })

test("the list below 900px pane width, the board from 900px, and back when it shrinks", () => {
  render(<TasksPane dir="/p" reload={() => {}} />)
  expect(screen.getByRole("heading", { name: "Tasks" })).toBeTruthy()
  expect(screen.queryByTestId("board")).toBeNull()

  width = 900
  act(() => resized())
  expect(screen.getByTestId("board")).toBeTruthy()
  expect(screen.queryByRole("heading", { name: "Tasks" })).toBeNull()

  width = 899
  act(() => resized())
  expect(screen.queryByTestId("board")).toBeNull()
  expect(screen.getByRole("heading", { name: "Tasks" })).toBeTruthy()
})
