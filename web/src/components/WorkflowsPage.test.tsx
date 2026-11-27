import { afterEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { WorkflowsPage } from "./WorkflowsPage"

const job = (ID: string, over: Record<string, unknown> = {}) => ({ ID, Status: "dev", Agent: "dev", Model: "", Prompt: "", Run: "", Uses: "", Needs: [], Outputs: [], OnFailure: "", MaxLoops: 0, ...over })
vi.mock("@/api", () => ({
  api: vi.fn(async () => [{ key: "feature", name: "feature", global: false, yaml: "", description: "", error: "", errorLine: 0, jobs: [job("build", { Run: "make test", Agent: "", Status: "review" }), job("test", { Prompt: "Fix the failing test", OnFailure: "build", MaxLoops: 2 })], inputs: {}, total: 0, running: 0, lastUsed: "", workplace: "folder", cleanup: "keep" }]),
  send: vi.fn(),
}))

afterEach(() => cleanup())

test("the selected workflow's header shows its workplace type and cleanup", async () => {
  // The flow canvas measures its size; jsdom has no ResizeObserver.
  vi.stubGlobal("ResizeObserver", class { observe() {} unobserve() {} disconnect() {} })
  render(<WorkflowsPage dir="/p" initial="feature" />)
  expect(await screen.findByText(/workplace folder · cleanup keep/)).toBeTruthy()
})

test("the flow shows each step as a card, with loops drawn as their own node", async () => {
  render(<WorkflowsPage dir="/p" initial="feature" />)
  expect(await screen.findByRole("button", { name: /^build/ })).toBeTruthy()
  expect(screen.getByRole("button", { name: /back to build/ })).toBeTruthy()
})

test("selecting a step shows its details", async () => {
  render(<WorkflowsPage dir="/p" initial="feature" />)
  fireEvent.click(await screen.findByRole("button", { name: /^test/ }))
  expect(screen.getByText("Fix the failing test")).toBeTruthy()
  expect(screen.getByRole("button", { name: /^test/ }).getAttribute("aria-pressed")).toBe("true")
})
