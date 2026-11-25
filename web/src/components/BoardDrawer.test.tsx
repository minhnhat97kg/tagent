/// <reference types="node" />
import { readFileSync } from "node:fs"
import path from "node:path"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, render, screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { BoardDrawer } from "./BoardDrawer"
import { DialogContent } from "@/components/ui/dialog"
import type { RunView as RunData, Task, Workflow } from "@/types"

const mocks = vi.hoisted(() => ({ api: vi.fn(), post: vi.fn() }))
vi.mock("@/api", () => ({ api: mocks.api, post: mocks.post, act: (p: Promise<unknown>) => p.catch(() => {}) }))
// The drawer only exists inside a Radix dialog; the tabs are what this test is about.
vi.mock("@/components/ui/dialog", () => ({
  Dialog: ({ children }: { children?: React.ReactNode }) => children,
  DialogContent: ({ children, className }: { children?: React.ReactNode; className?: string }) => <div className={className} data-slot="dialog-content">{children}</div>,
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

test("Activity holds the conversation with a collapsed composer on top", async () => {
  mount()
  await userEvent.click(screen.getByRole("tab", { name: "Activity" }))
  const activity = screen.getByRole("tabpanel")
  expect(within(activity).getByText("first comment")).toBeTruthy()
  expect(within(activity).queryByRole("textbox", { name: "Comment" })).toBeNull()
  await userEvent.click(within(activity).getByRole("button", { name: "Add a comment…" }))
  expect(within(activity).getByRole("textbox", { name: "Comment" })).toBeTruthy()
  expect(within(activity).queryByText("Goal")).toBeNull()
})

test("Ctrl+Enter saves the comment; Escape collapses an empty composer", async () => {
  mocks.post.mockResolvedValue({})
  const reload = vi.fn()
  render(<BoardDrawer task={task} statuses={["todo", "build", "done"]} roles={["builder"]} role="builder" dir="/repo" reload={reload} workflows={[workflow]} />)
  await userEvent.click(screen.getByRole("tab", { name: "Activity" }))
  await userEvent.click(screen.getByRole("button", { name: "Add a comment…" }))
  const box = screen.getByRole("textbox", { name: "Comment" })
  await userEvent.type(box, "looks good{Control>}{Enter}{/Control}")
  await waitFor(() => expect(screen.queryByRole("textbox", { name: "Comment" })).toBeNull())
  expect(reload).toHaveBeenCalled()
  expect(vi.mocked(fetch).mock.calls.some(([, init]) => (init as RequestInit | undefined)?.method === "POST")).toBe(true)
  await userEvent.click(screen.getByRole("button", { name: "Add a comment…" }))
  await userEvent.click(screen.getByRole("textbox", { name: "Comment" }))
  await userEvent.keyboard("{Escape}")
  expect(screen.queryByRole("textbox", { name: "Comment" })).toBeNull()
})

test("the M key opens the composer on Activity", async () => {
  mount()
  await userEvent.click(screen.getByRole("tab", { name: "Activity" }))
  await userEvent.click(document.body)
  await userEvent.keyboard("m")
  expect(screen.getByRole("textbox", { name: "Comment" })).toBeTruthy()
})

test("typing @ in the composer suggests roles and inserts the one picked", async () => {
  mount()
  await userEvent.click(screen.getByRole("tab", { name: "Activity" }))
  await userEvent.click(screen.getByRole("button", { name: "Add a comment…" }))
  await userEvent.type(screen.getByRole("textbox", { name: "Comment" }), "@bu")
  const list = screen.getByRole("listbox", { name: "Mention role" })
  await userEvent.click(within(list).getByRole("option", { name: "@builder" }))
  expect((screen.getByRole("textbox", { name: "Comment" }) as HTMLTextAreaElement).value).toBe("@builder ")
})

test("Activity filters hide history or comments, and sort order is remembered", async () => {
  mount()
  await userEvent.click(screen.getByRole("tab", { name: "Activity" }))
  const activity = screen.getByRole("tabpanel")
  expect(await within(activity).findByText("started build")).toBeTruthy()
  await userEvent.click(within(activity).getByRole("button", { name: "Comments" }))
  expect(within(activity).queryByText("started build")).toBeNull()
  expect(within(activity).getByText("first comment")).toBeTruthy()
  await userEvent.click(within(activity).getByRole("button", { name: "History" }))
  expect(within(activity).queryByText("first comment")).toBeNull()
  await userEvent.click(within(activity).getByRole("button", { name: "All" }))
  // Newest first by default: the comment (10:00) is above the history (09:10).
  const texts = within(activity).getAllByRole("listitem").map(li => li.textContent ?? "")
  expect(texts[0]).toContain("first comment")
  await userEvent.click(within(activity).getByRole("button", { name: "Newest first" }))
  expect(localStorage.getItem("board.activitySort")).toBe("oldest")
  expect(within(activity).getAllByRole("listitem")[0]!.textContent).toContain("started build")
  localStorage.removeItem("board.activitySort")
})

test("a role that is working on a mention shows 'replying…' until its reply lands", async () => {
  const mention = { ...task, comments: [{ id: "c1", author: "user", body: "@builder take a look", created: "2026-11-23T10:00:00Z" }] }
  const { rerender } = render(<BoardDrawer task={mention} statuses={["todo", "build", "done"]} roles={["builder"]} role="builder" dir="/repo" reload={() => {}} workflows={[workflow]} project={{ dir: "/repo", tasks: [], roles: [], statuses: [], running: { "30": ["builder"] } }} />)
  await userEvent.click(screen.getByRole("tab", { name: "Activity" }))
  expect(screen.getByText("builder replying…")).toBeTruthy()
  rerender(<BoardDrawer task={{ ...mention, comments: [...mention.comments, { id: "c2", author: "builder", body: "done", created: "2026-11-23T10:05:00Z" }] }} statuses={["todo", "build", "done"]} roles={["builder"]} role="builder" dir="/repo" reload={() => {}} workflows={[workflow]} project={{ dir: "/repo", tasks: [], roles: [], statuses: [], running: { "30": ["builder"] } }} />)
  await waitFor(() => expect(screen.queryByText(/replying/)).toBeNull())
})

test("Reply on a role's comment opens the composer with the mention", async () => {
  mount()
  await userEvent.click(screen.getByRole("tab", { name: "Activity" }))
  await userEvent.click(screen.getByRole("button", { name: "Reply" }))
  expect((screen.getByRole("textbox", { name: "Comment" }) as HTMLTextAreaElement).value).toBe("@builder ")
})

test("the header puts Start, Edit and Delete in one row and Start runs the chosen flow", async () => {
  mount()
  const row = document.querySelector(".drawer-actions") as HTMLElement
  expect(within(row).getByRole("button", { name: "Edit" })).toBeTruthy()
  expect(within(row).getByRole("button", { name: "Delete" })).toBeTruthy()
  expect(document.querySelector(".drawer-start")).toBeNull()
  // The task is in build with a workflow, so there is nothing to start.
  expect(within(row).queryByRole("button", { name: "Start" })).toBeNull()
})

test("the workflow is picked in Details and Start only starts", async () => {
  const todo = { ...task, status: "todo", workflow: undefined, comments: [] }
  render(<BoardDrawer task={todo} statuses={["todo", "build", "done"]} roles={["builder"]} role="" dir="/repo" reload={() => {}} workflows={[workflow]} />)
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Task workflow" }), "feature")
  expect(mocks.post).toHaveBeenCalledWith("/api/tasks", { Dir: "/repo", ID: "30", Op: "workflow", Workflow: "feature" })
  const row = document.querySelector(".drawer-actions") as HTMLElement
  expect(within(row).queryByRole("button", { name: "Choose workflow" })).toBeNull()
  await userEvent.click(within(row).getByRole("button", { name: "Start" }))
  expect(mocks.post).toHaveBeenCalledWith("/api/tasks", { Dir: "/repo", ID: "30", Op: "start", Status: "" })
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
  expect(mocks.api).not.toHaveBeenCalledWith(expect.stringContaining("/run"))
})

test("arrow keys walk the tabs, skip a disabled Chat, and only the active tab is in the tab order", async () => {
  mocks.api.mockImplementation(async (path: string) => path.startsWith("/api/tickets/30/chat")
    ? { key: "scope#30", pane: "%3", harness: "codex", available: false, reason: "Chat needs a claude or pi agent, but %3 runs codex." }
    : run)
  const tablist = mount()
  await userEvent.click(screen.getByRole("tab", { name: "Details" }))
  await userEvent.keyboard("{ArrowRight}")
  expect(screen.getByRole("tab", { name: "Activity" }).getAttribute("aria-selected")).toBe("true")
  expect(screen.getByRole("tab", { name: "Details" }).getAttribute("tabIndex")).toBe("-1")
  await userEvent.keyboard("{ArrowRight}")
  expect(screen.getByRole("tab", { name: "Run" }).getAttribute("aria-selected")).toBe("true")
  await userEvent.keyboard("{ArrowRight}")
  expect(screen.getByRole("tab", { name: "Details" }).getAttribute("aria-selected")).toBe("true")
  // Backwards also skips the disabled Chat: the cursor goes where the key points.
  await userEvent.keyboard("{ArrowLeft}")
  expect(screen.getByRole("tab", { name: "Run" }).getAttribute("aria-selected")).toBe("true")
  await userEvent.keyboard("{End}")
  expect(screen.getByRole("tab", { name: "Run" }).getAttribute("aria-selected")).toBe("true")
  await waitFor(() => expect(within(tablist).getAllByRole("tab")).toHaveLength(4))
})

test("Chat is disabled with the reason when the ticket's pane cannot be read", async () => {
  mocks.api.mockImplementation(async (path: string) => path.startsWith("/api/tickets/30/chat")
    ? { key: "scope#30", pane: "%3", harness: "codex", available: false, reason: "Chat needs a claude or pi agent, but %3 runs codex." }
    : run)
  mount()
  const chat = screen.getByRole("tab", { name: "Chat" })
  await waitFor(() => expect(chat.hasAttribute("disabled")).toBe(true))
  expect(screen.getByRole("status").textContent).toContain("Chat needs a claude or pi agent")
  await userEvent.click(chat)
  expect(screen.getByRole("tab", { name: "Details" }).getAttribute("aria-selected")).toBe("true")
})

test("Chat starts the ticket's pane and a message can be saved as a comment", async () => {
  const reload = vi.fn()
  mocks.api.mockImplementation(async (path: string) => {
    if (path.startsWith("/api/tickets/30/chat")) return { key: "scope#30", pane: "", harness: "", status: "", available: true }
    return { harness: "claude", status: "idle", items: [{ id: "a.0", role: "assistant", text: "Keep the queue." }], cursor: "a.0", reset: true, more: false, queued: [] }
  })
  mocks.post.mockResolvedValue({ key: "scope#30", pane: "%7", harness: "claude", status: "idle", available: true, created: true })
  render(<BoardDrawer task={task} statuses={["todo", "build", "done"]} roles={["builder"]} role="builder" dir="/repo" reload={reload} workflows={[workflow]} />)
  await userEvent.click(screen.getByRole("tab", { name: "Chat" }))
  await userEvent.click(await screen.findByRole("button", { name: /start chat/i }))
  expect(mocks.post).toHaveBeenCalledWith("/api/tickets/30/chat?dir=%2Frepo", {})
  expect(await screen.findByText("Keep the queue.")).toBeTruthy()
  await userEvent.click(screen.getByRole("button", { name: "Save to ticket" }))
  expect(mocks.post).toHaveBeenCalledWith("/api/tasks", { Dir: "/repo", Op: "comment", ID: "30", Text: "Keep the queue." })
  expect(reload).toHaveBeenCalled()
})

test("a reply notification opens on Activity with the draft ready", () => {
  render(<BoardDrawer task={task} statuses={["todo", "build", "done"]} roles={["builder"]} role="builder" dir="/repo" reload={() => {}} workflows={[workflow]} initialReply="@builder " />)
  expect(screen.getByRole("tab", { name: "Activity" }).getAttribute("aria-selected")).toBe("true")
  expect((screen.getByRole("textbox", { name: "Comment" }) as HTMLTextAreaElement).value).toBe("@builder ")
})

test("Details shows usage as one line that opens the per-job breakdown", async () => {
  const c = (n: number) => ({ Input: n, Output: 0, CacheRead: 0, CacheWrite: 0, Total: n })
  mocks.api.mockImplementation(async (path: string) => path.includes("/usage") ? { jobs: [
    { role: "dev", harness: "pi", model: "gpt", effort: "high", loops: 2, elapsed: 5e9, count: c(1500), has: true, live: false },
    { role: "doc", harness: "antigravity", model: "", effort: "", loops: 1, elapsed: 1e9, count: c(0), has: false, live: false },
  ], models: [], total: c(1500), partial: true, elapsed: 6e9, loops: 3 } : run)
  mount()
  const summary = await screen.findByRole("button", { name: /Usage/ })
  expect(summary.getAttribute("aria-expanded")).toBe("false")
  expect(screen.getByText("partial")).toBeTruthy()
  expect(screen.getAllByText("1.5k")).toHaveLength(2) // summary and the legend
  expect(screen.getByRole("img", { name: "gpt 100%" })).toBeTruthy() // per-model bar; jobs without tokens are left out
  expect(screen.queryByText("n/a")).toBeNull()
  await userEvent.click(summary)
  expect(summary.getAttribute("aria-expanded")).toBe("true")
  expect(screen.getAllByText("gpt").length).toBeGreaterThan(1) // legend and the job line
  expect(screen.getByText("n/a")).toBeTruthy()
})

const others: Task[] = [
  { id: "31", title: "API", text: "API", status: "build", assignees: [], comments: [], created: "2026-11-23T09:00:00Z", updated: "2026-11-23T09:00:00Z" },
  { id: "32", title: "Docs", text: "Docs", status: "done", assignees: [], comments: [], created: "2026-11-23T09:00:00Z", updated: "2026-11-23T09:00:00Z" },
  { id: "33", title: "Login page", text: "Login page", status: "todo", assignees: [], comments: [], created: "2026-11-23T09:00:00Z", updated: "2026-11-23T09:00:00Z" },
]

test("a dependency is picked from the ticket list, not typed, and removed with its button", async () => {
  mocks.post.mockResolvedValue({})
  const withDep = { ...task, needs: ["31"] }
  render(<BoardDrawer task={withDep} statuses={["todo", "build", "done"]} roles={["builder"]} role="builder" dir="/repo" reload={() => {}} workflows={[workflow]}
    project={{ dir: "/repo", tasks: [withDep, ...others], roles: [], statuses: [], running: {} }} />)
  expect(screen.getByRole("button", { name: /API/ })).toBeTruthy()
  await userEvent.click(screen.getByRole("button", { name: "Add" }))
  const picker = screen.getByRole("list", { name: "Tickets to depend on" })
  // This ticket and the picked one are hidden; the done ticket sorts last.
  const names = within(picker).getAllByRole("button").map(b => b.textContent ?? "")
  expect(names).toHaveLength(2)
  expect(names[0]).toContain("Login page")
  expect(names[1]).toContain("Docs")
  await userEvent.type(screen.getByRole("textbox", { name: "Filter tickets" }), "login")
  expect(within(picker).getAllByRole("button")).toHaveLength(1)
  await userEvent.click(within(picker).getByRole("button", { name: /Login page/ }))
  expect(mocks.post).toHaveBeenCalledWith("/api/tasks", { Dir: "/repo", Op: "needs", ID: "30", Needs: ["31", "33"] })
  await userEvent.click(screen.getByRole("button", { name: "Remove dependency #31" }))
  expect(mocks.post).toHaveBeenCalledWith("/api/tasks", { Dir: "/repo", Op: "needs", ID: "30", Needs: [] })
})

// Layout contract from board.css: only the active tab body scrolls; the drawer, its header and the tab strip do not.
test("a long Run scrolls only the tab body; the header and tab strip stay put", async () => {
  const style = document.createElement("style")
  style.textContent = readFileSync(path.resolve(import.meta.dirname, "board.css"), "utf8")
  document.head.append(style)
  try {
    const entry = run.entries![0]
    mocks.api.mockResolvedValue({ ...run, entries: Array.from({ length: 60 }, (_, i) => ({ ...entry, seq: i + 1, comment: `entry ${i + 1}` })) })
    // Board.tsx wraps the drawer in the Radix content that carries the board-drawer class.
    render(<DialogContent className="board-drawer translate-x-0 translate-y-0 sm:max-w-none"><BoardDrawer task={task} statuses={["todo", "build", "done"]} roles={["builder"]} role="builder" dir="/repo" reload={() => {}} workflows={[workflow]} /></DialogContent>)
    await userEvent.click(screen.getByRole("tab", { name: "Run" }))
    await screen.findByText("entry 60")
    const drawer = document.querySelector<HTMLElement>('[data-slot="dialog-content"]')!
    const panel = screen.getByRole("tabpanel")
    const tabs = screen.getByRole("tablist")
    expect(getComputedStyle(drawer).overflowY).not.toBe("auto")
    expect(getComputedStyle(panel).overflowY).toBe("auto")
    expect(getComputedStyle(panel).flexShrink).not.toBe("0")
    expect(getComputedStyle(tabs).flexShrink).toBe("0")
  } finally {
    style.remove()
  }
})
