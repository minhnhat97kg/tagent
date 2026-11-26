import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react"
import { WorkflowsPane } from "./WorkflowsPane"

vi.mock("@/api", () => ({ api: vi.fn(async () => [{ key: "feature", name: "feature", global: false, description: "Builds it", running: 2, jobs: [] }]) }))
// The editor is covered by its own tests; here it only has to show up and report a draft.
vi.mock("@/components/WorkflowsPage", () => ({ WorkflowsPage: ({ onDirty }: { onDirty?: (d: boolean) => void }) => <button onClick={() => onDirty?.(true)}>editor</button> }))

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

test("below 900px the pane lists the workflows; from 900px it is the editor", async () => {
  render(<WorkflowsPane dir="/p" />)
  expect((await screen.findByRole("link", { name: /feature/ })).getAttribute("href")).toBe("#/workflows/feature")
  width = 900
  act(() => resized())
  expect(screen.queryByRole("link", { name: /feature/ })).toBeNull()
  expect(screen.getByText("editor")).toBeTruthy()
})

test("a dirty editor stays up when the pane narrows", () => {
  width = 900
  render(<WorkflowsPane dir="/p" />)
  fireEvent.click(screen.getByText("editor"))
  width = 500
  act(() => resized())
  expect(screen.getByText("editor")).toBeTruthy()
})
