import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { AgentChat, cleanAnswer as cleanAnswerOf } from "./AgentChat"
import type { Feed } from "@/lib/transcript"

const mocks = vi.hoisted(() => ({ api: vi.fn(), post: vi.fn() }))
vi.mock("@/api", () => ({ api: mocks.api, post: mocks.post }))

function feed(over: Partial<Feed> = {}): Feed {
  return { harness: "claude", status: "idle", items: [], cursor: "", reset: false, more: false, queued: [], ...over }
}

beforeEach(() => {
  mocks.api.mockReset().mockResolvedValue(feed({
    status: "running",
    items: [
      { id: "u.0", role: "user", text: "fix the board" },
      { id: "a.0", role: "tool", tool: { name: "Edit", label: "Edit Board.tsx", callId: "t1", input: "{}" } },
      { id: "r.0", role: "tool", tool: { callId: "t1", output: "edited" } },
      { id: "a.1", role: "assistant", text: "Done." },
    ],
    cursor: "a.1",
  }))
  mocks.post.mockReset().mockResolvedValue({ queued: false })
  Element.prototype.scrollTo = vi.fn()
})
afterEach(cleanup)

function mount(onOpenTerminal = vi.fn(), onUnavailable = vi.fn()) {
  render(<AgentChat pane="%1" onOpenTerminal={onOpenTerminal} onUnavailable={onUnavailable} />)
  return screen.getByRole("textbox", { name: "Message agent" }) as HTMLTextAreaElement
}

test("renders messages and merges a tool result into its card", async () => {
  mount()
  expect(await screen.findByText("Done.")).toBeTruthy()
  expect(screen.getByText("fix the board")).toBeTruthy()
  expect(screen.getByText("Edit Board.tsx")).toBeTruthy()
  expect(screen.getByText("edited")).toBeTruthy()
  expect(screen.getByRole("status").textContent).toContain("Working")
  expect(mocks.api).toHaveBeenCalledWith("/api/agents/%251/transcript")
})

test("follows the cursor, and waits between polls", async () => {
  mount()
  await screen.findByText("Done.")
  await waitFor(() => expect(mocks.api.mock.calls.length).toBeGreaterThan(1), { timeout: 3000 })
  expect(mocks.api.mock.calls[1][0]).toBe("/api/agents/%251/transcript?after=a.1")
})

test("Enter sends, Shift+Enter keeps a newline, and IME composition does not send", async () => {
  const input = mount()
  await screen.findByText("Done.")
  await userEvent.type(input, "first{Shift>}{Enter}{/Shift}second")
  expect(input.value).toBe("first\nsecond")
  expect(mocks.post).not.toHaveBeenCalled()
  fireEvent.keyDown(input, { key: "Enter", isComposing: true })
  expect(mocks.post).not.toHaveBeenCalled()
  await userEvent.keyboard("{Enter}")
  expect(mocks.post).toHaveBeenCalledWith("/api/pane/send", { pane: "%1", data: "first\nsecond" })
  await waitFor(() => expect(input.value).toBe(""))
})

test("a busy agent queues the message and the queue is shown", async () => {
  mocks.post.mockResolvedValue({ queued: true })
  const input = mount()
  await screen.findByText("Done.")
  await userEvent.type(input, "after this{Enter}")
  expect(await screen.findByText("Queued until the agent is idle (1):")).toBeTruthy()
  expect(screen.getByText("after this")).toBeTruthy()
})

test("an approval shows the excerpt and only opens Terminal, never answers", async () => {
  mocks.api.mockResolvedValue(feed({ status: "waiting", screen: "Allow Bash?\n1. Yes\n2. No", items: [] }))
  const onOpenTerminal = vi.fn()
  mount(onOpenTerminal)
  expect(await screen.findByText(/Allow Bash\?/)).toBeTruthy()
  expect(screen.queryByRole("button", { name: /yes|no/i })).toBeNull()
  await userEvent.click(screen.getByRole("button", { name: "Open in Terminal" }))
  expect(onOpenTerminal).toHaveBeenCalled()
  expect(mocks.post).not.toHaveBeenCalled()
})

test("an unsupported or unreadable transcript tells the parent", async () => {
  mocks.api.mockResolvedValue(feed({ unsupported: true }))
  const onUnavailable = vi.fn()
  mount(undefined, onUnavailable)
  await waitFor(() => expect(onUnavailable).toHaveBeenCalled())
  expect(await screen.findByText(/not available for this agent/)).toBeTruthy()
})

test("shows an AskUserQuestion with its options and answers by the option number", async () => {
  const input = JSON.stringify({ questions: [{ question: "Which chat?", options: [{ label: "Both" }, { label: "Agent", description: "AgentChat" }] }] })
  mocks.api.mockReset().mockResolvedValue(feed({ status: "waiting", screen: "raw screen", items: [
    { id: "q.0", role: "tool", tool: { name: "AskUserQuestion", label: "AskUserQuestion", callId: "q1", input } },
  ] }))
  mount()
  expect(await screen.findByText("Which chat?")).toBeTruthy()
  expect(screen.queryByText("raw screen")).toBeNull()
  await userEvent.click(screen.getByRole("button", { name: /2\s*Agent/ }))
  expect(mocks.post).toHaveBeenCalledWith("/api/pane/input", { pane: "%1", data: "2" })
})

test("a pending question read off the screen is a card whose option is clickable", async () => {
  mocks.api.mockReset().mockResolvedValue(feed({ status: "waiting", items: [], ask: { header: "Pick", question: "Pick again", other: true, options: [{ n: 1, label: "Red", description: "Choose Red" }, { n: 2, label: "Green" }] } }))
  mount()
  expect(await screen.findByText("Pick again")).toBeTruthy()
  expect(screen.queryByText(/Approval needed/)).toBeNull()
  await userEvent.click(screen.getByRole("button", { name: /2\s*Green/ }))
  expect(mocks.post).toHaveBeenCalledWith("/api/pane/input", { pane: "%1", data: "2" })
  expect(screen.getByRole("button", { name: /Answer another way in Terminal/ })).toBeTruthy()
})

test("multi-select ticks by number, a typed answer picks the Type something row, review submits", async () => {
  mocks.api.mockReset().mockResolvedValue(feed({ status: "waiting", items: [], ask: { question: "Pick toppings", multi: true, otherN: 3, options: [{ n: 1, label: "Cheese", checked: true }, { n: 2, label: "Ham" }] } }))
  mount()
  expect((await screen.findByRole("checkbox", { name: /Cheese/ })).getAttribute("aria-checked")).toBe("true")
  expect(screen.getByRole("checkbox", { name: /Ham/ }).getAttribute("aria-checked")).toBe("false")
  await userEvent.click(screen.getByRole("checkbox", { name: /Ham/ }))
  expect(mocks.post).toHaveBeenCalledWith("/api/pane/input", { pane: "%1", data: "2" })
  await userEvent.click(screen.getByRole("button", { name: /Review & submit/ }))
  expect(mocks.post).toHaveBeenCalledWith("/api/pane/input", { pane: "%1", data: "\u001b[C" })
  await userEvent.type(screen.getByLabelText("Type your own answer"), "Tuna")
  await userEvent.click(screen.getByRole("button", { name: "Send answer" }))
  await waitFor(() => expect(mocks.post).toHaveBeenCalledWith("/api/pane/input", { pane: "%1", data: "\r" }))
  const sent = mocks.post.mock.calls.map(c => (c[1] as { data: string }).data)
  expect(sent.slice(-3)).toEqual(["3", "Tuna", "\r"])
})

test("an answered question collapses to one line and expands on tap", async () => {
  const input = JSON.stringify({ questions: [{ question: "Which chat?", options: [{ label: "Both" }, { label: "Agent" }] }] })
  mocks.api.mockReset().mockResolvedValue(feed({ items: [
    { id: "q.0", role: "tool", tool: { name: "AskUserQuestion", label: "AskUserQuestion", callId: "q1", input } },
    { id: "q.1", role: "tool", tool: { callId: "q1", output: 'Your questions have been answered: "Which chat?"="Agent". You can now continue' } },
  ] }))
  mount()
  const row = await screen.findByRole("button", { name: /Question: Which chat\?/ })
  expect(row.textContent).toContain("Agent")
  expect(row.textContent).not.toContain("answered")
  expect(screen.queryByText("Both")).toBeNull()
  await userEvent.click(row)
  expect(screen.getByText("Both")).toBeTruthy()
  expect(cleanAnswerOf('The user answered: "A?"="x y", "B?"="z". Read the answers carefully')).toBe("x y · z")
})

test("offers the harness's slash commands and completes one", async () => {
  const input = mount()
  await screen.findByText("Done.")
  await userEvent.type(input, "/comp")
  expect(screen.getByRole("listbox", { name: "Commands" })).toBeTruthy()
  await userEvent.keyboard("{Enter}")
  expect(input.value).toBe("/compact")
  expect(mocks.post).not.toHaveBeenCalled()
})

test("shows the model and switches it with /model", async () => {
  mocks.api.mockReset().mockResolvedValue(feed({ items: [{ id: "a.0", role: "assistant", text: "Hi", model: "claude-opus-5-5" }] }))
  mount()
  await userEvent.click(await screen.findByRole("button", { name: "Model: claude-opus-5-5" }))
  await userEvent.click(await screen.findByRole("menuitem", { name: "haiku" }))
  expect(mocks.post).toHaveBeenCalledWith("/api/pane/send", { pane: "%1", data: "/model haiku" })
})

test("folds consecutive tool calls into one line and keeps the status in the bar", async () => {
  mocks.api.mockReset().mockResolvedValue(feed({ status: "running", queued: ["/model haiku"], items: [
    { id: "a.0", role: "tool", tool: { name: "Bash", label: "Bash ls", callId: "t1", input: "{}" } },
    { id: "r.0", role: "tool", tool: { callId: "t1", output: "ok" } },
    { id: "a.1", role: "tool", tool: { name: "Bash", label: "Bash pwd", callId: "t2", input: "{}" } },
    { id: "r.1", role: "tool", tool: { callId: "t2", output: "ok" } },
    { id: "a.2", role: "tool", tool: { name: "Read", label: "Read a.ts", callId: "t3", input: "{}" } },
    { id: "m.0", role: "assistant", text: "| a | b |\n|---|---|\n| 1 | 2 |" },
  ] }))
  mount()
  expect(await screen.findByText("Bash ×2 · Read")).toBeTruthy()
  expect(screen.getByText("1 queued")).toBeTruthy()
  expect((await screen.findAllByRole("table")).length).toBe(1)
})
