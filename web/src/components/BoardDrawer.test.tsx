import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { BoardDrawer } from "./BoardDrawer"
import type { RunView as RunData, Task, Workflow } from "@/types"

const mocks = vi.hoisted(() => ({ api: vi.fn(), post: vi.fn() }))
vi.mock("@/api", () => ({ api: mocks.api, post: mocks.post, act: (p: Promise<unknown>) => p.catch(() => {}) }))
// The drawer only exists inside a Radix dialog; the tabs are what this test is about.
vi.mock("@/components/ui/dialog", () => ({
  Dialog: ({ children }: { children?: React.ReactNode }) => children,
  DialogContent: ({ children }: { children?: React.ReactNode }) => children,
  DialogTitle: ({ children }: { children?: React.ReactNode }) => children,
  DialogDescription: () => null,
}))

const task: Task = {
  id: "30",
  title: "Side panel",
  text: "Side panel\nGoal\nThe panel has tabs.",
  status: "build",
  workflow: "feature",
  assignees: ["builder"],
  created: "2026-11-23T09:00:00Z",
  updated: "2026-11-23T10:00:00Z",
  comments: [{ id: "c1", author: "builder", body: "first comment", created: "2026-11-23T10:00:00Z" }],
}

const workflow: Workflow = { name: "feature", description: "", global: false, file: "feature.yaml", error: "", jobs: [{ id: "build", status: "build", agent: "builder", run: "" }] }

const run: RunData = {
  ticket: "30",
  title: "Side panel",
  workflow: "feature",
  status: "build",
  started: "2026-11-23T09:10:00Z",
  total: 12 * 60 * 1e9,
  entries: [{ seq: 1, step: "build", loop: 1, role: "builder", start: "2026-11-23T09:10:00Z", duration: 12 * 60 * 1e9, result: "pass", comment: "built the panel" }],
  flow: [{ job: "build", status: "build", count: 1, current: true }],
}

beforeEach(() => {
  mocks.api.mockReset().mockResolvedValue(run)
  mocks.post.mockReset().mockResolvedValue({})
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, json: async () => [] })))
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

function mount() {
  render(<BoardDrawer task={task} statuses={["todo", "build", "done"]} roles={["builder"]} role="builder" dir="/repo" reload={() => {}} workflows={[workflow]} />)
  return screen.getByRole("tablist", { name: "Ticket panel" })
}

test("opens on Details with the ticket facts", async () => {
  mount()
  expect(screen.getByRole("tab", { name: "Details" }).getAttribute("aria-selected")).toBe("true")
  const details = screen.getByRole("tabpanel")
  expect(within(details).getByText("feature")).toBeTruthy()
  expect(within(details).getByText(/The panel has tabs\./)).toBeTruthy()
  expect(within(details).queryByRole("textbox", { name: "Comment" })).toBeNull()
})

test("Activity holds the conversation and the comment form", async () => {
  mount()
  await userEvent.click(screen.getByRole("tab", { name: "Activity" }))
  const activity = screen.getByRole("tabpanel")
  expect(screen.getByRole("tab", { name: "Activity" }).getAttribute("aria-selected")).toBe("true")
  expect(within(activity).getByText("first comment")).toBeTruthy()
  expect(within(activity).getByRole("textbox", { name: "Comment" })).toBeTruthy()
  expect(within(activity).queryByText("Goal")).toBeNull()
})

test("Run reuses the run view inside the panel", async () => {
  mount()
  await userEvent.click(screen.getByRole("tab", { name: "Run" }))
  const panel = screen.getByRole("tabpanel")
  expect(await within(panel).findByText("built the panel")).toBeTruthy()
  expect(within(panel).getByText("pass")).toBeTruthy()
  expect(within(panel).getByText("1 entry")).toBeTruthy()
  expect(mocks.api).toHaveBeenCalledWith(expect.stringContaining("/api/tasks/30/run"))
})

test("a ticket without a workflow says so on the Run tab", async () => {
  render(<BoardDrawer task={{ ...task, workflow: undefined }} statuses={["todo", "done"]} roles={[]} role="" dir="/repo" reload={() => {}} workflows={[]} />)
  await userEvent.click(screen.getByRole("tab", { name: "Run" }))
  expect(within(screen.getByRole("tabpanel")).getByText(/no workflow/i)).toBeTruthy()
  expect(mocks.api).not.toHaveBeenCalled()
})

test("arrow keys walk the tabs and only the active tab is in the tab order", async () => {
  const tablist = mount()
  await userEvent.click(screen.getByRole("tab", { name: "Details" }))
  await userEvent.keyboard("{ArrowRight}")
  expect(screen.getByRole("tab", { name: "Activity" }).getAttribute("aria-selected")).toBe("true")
  expect(screen.getByRole("tab", { name: "Details" }).getAttribute("tabIndex")).toBe("-1")
  await userEvent.keyboard("{ArrowRight}")
  expect(screen.getByRole("tab", { name: "Run" }).getAttribute("aria-selected")).toBe("true")
  await userEvent.keyboard("{ArrowRight}")
  expect(screen.getByRole("tab", { name: "Details" }).getAttribute("aria-selected")).toBe("true")
  await userEvent.keyboard("{End}")
  expect(screen.getByRole("tab", { name: "Run" }).getAttribute("aria-selected")).toBe("true")
  await waitFor(() => expect(within(tablist).getAllByRole("tab")).toHaveLength(3))
})

test("a reply notification opens on Activity with the draft ready", () => {
  render(<BoardDrawer task={task} statuses={["todo", "build", "done"]} roles={["builder"]} role="builder" dir="/repo" reload={() => {}} workflows={[workflow]} initialReply="@builder " />)
  expect(screen.getByRole("tab", { name: "Activity" }).getAttribute("aria-selected")).toBe("true")
  expect((screen.getByRole("textbox", { name: "Comment" }) as HTMLTextAreaElement).value).toBe("@builder ")
})
