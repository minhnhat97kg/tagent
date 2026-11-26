import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { AgentsPanel } from "./AgentsPanel"
import { OverviewPanel } from "./OverviewPanel"
import type { Agent, Overview } from "@/types"

// Heights are read on mount; the tiers apply only on a desktop window.
const height = (h: number) => vi.spyOn(Element.prototype, "clientHeight", "get").mockReturnValue(h)
beforeEach(() => {
  globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver
  window.matchMedia = ((q: string) => ({ matches: true, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
})
afterEach(() => { cleanup(); vi.restoreAllMocks() })

const agents = [
  { pane: "%1", name: "a", agent: "claude", status: "running", session: "", window: "", path: "/p", project: "p", prompt: "one" },
  { pane: "%2", name: "b", agent: "claude", status: "waiting", session: "", window: "", path: "/p", project: "p", prompt: "two" },
] as Agent[]
const list = () => <AgentsPanel ordered={agents} byTask={false} onToggleTask={() => {}} onSelect={() => {}} />

test("agents: under 120px only the counts show; from 120px the list does", () => {
  height(119)
  const { unmount } = render(list())
  expect(screen.getByText("2 agents, 1 waiting, 1 working")).toBeTruthy()
  expect(screen.queryByRole("button", { name: /one/ })).toBeNull()
  unmount()
  height(120)
  render(list())
  expect(screen.queryByText("2 agents, 1 waiting, 1 working")).toBeNull()
  expect(screen.getAllByRole("button").length).toBeGreaterThan(0)
})

test("agents: an unmeasured height (0) is the normal list", () => {
  height(0)
  render(list())
  expect(screen.queryByText("2 agents, 1 waiting, 1 working")).toBeNull()
})

const overview = { dir: "", project: "", attention: { needed: false, text: "" }, providers: [{ name: "Claude", usedPercent: 40, weeklyPercent: 10, stale: false }], tokens: { today: 1234, total7d: 0, sparkline: "", days: [{ date: "2026-11-25", tokens: 5 }] } } as Overview

test("usage: under 200px the quota meters stay and the token spend goes", () => {
  height(199)
  const { unmount } = render(<OverviewPanel overview={overview} />)
  expect(screen.getByText("5h")).toBeTruthy()
  expect(screen.queryByText("Tokens today")).toBeNull()
  unmount()
  height(200)
  render(<OverviewPanel overview={overview} />)
  expect(screen.getByText("Tokens today")).toBeTruthy()
})

// The Tasks pane follows its own width: a wide one is the board.
test("a wide Tasks pane renders the board", async () => {
  vi.spyOn(Element.prototype, "clientWidth", "get").mockReturnValue(1200)
  const { TasksPane } = await import("./TasksPane")
  render(<TasksPane dir="/p" reload={() => {}} />)
  expect(document.querySelector("section.task-board")).toBeTruthy()
})

test("usage: harness rows show their value or an error, and a hidden row stays hidden on this device", async () => {
  localStorage.clear()
  const user = userEvent.setup()
  const withRows = { ...overview, harnesses: [
    { harness: "Claude API", label: "month", usedPercent: 61, error: false },
    { harness: "Claude API", label: "credits", usedPercent: null, error: true },
  ] }
  const { unmount } = render(<OverviewPanel overview={withRows} />)
  expect(screen.getByTitle("month: 61% used")).toBeTruthy()
  expect(screen.getByText("error")).toBeTruthy()
  await user.click(screen.getByRole("button", { name: "Choose usage to show" }))
  await user.click(screen.getByRole("menuitemcheckbox", { name: "month" }))
  await user.click(screen.getByRole("menuitemcheckbox", { name: "5h and 7d limits" })) // the menu stays open
  expect(screen.queryByTitle("month: 61% used")).toBeNull()
  expect(screen.queryByText("5h")).toBeNull()
  unmount()
  render(<OverviewPanel overview={withRows} />)
  expect(screen.queryByTitle("month: 61% used")).toBeNull()
  expect(screen.queryByText("5h")).toBeNull()
  expect(screen.getByText("credits")).toBeTruthy()
})
