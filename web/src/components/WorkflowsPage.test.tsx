import { afterEach, expect, test, vi } from "vitest"
import { cleanup, render, screen } from "@testing-library/react"
import { WorkflowsPage } from "./WorkflowsPage"

vi.mock("@/api", () => ({
  api: vi.fn(async () => [{ key: "feature", name: "feature", global: false, yaml: "", description: "", error: "", errorLine: 0, jobs: [], inputs: {}, total: 0, running: 0, lastUsed: "", workplace: "folder", cleanup: "keep" }]),
  send: vi.fn(),
}))

afterEach(() => cleanup())

test("the selected workflow's header shows its workplace type and cleanup", async () => {
  // The flow chart measures its cards; jsdom has no ResizeObserver.
  vi.stubGlobal("ResizeObserver", class { observe() {} unobserve() {} disconnect() {} })
  render(<WorkflowsPage dir="/p" initial="feature" />)
  expect(await screen.findByText(/workplace folder · cleanup keep/)).toBeTruthy()
})
