import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react"
import { RoutingPage } from "./RoutingPage"
import { flow, parseRef, reorder, withUntil } from "@/lib/routing"
import { enc } from "@/hooks/useRoute"
import type { RoutingCandidate, RoutingView } from "@/types"

const mocks = vi.hoisted(() => ({ api: vi.fn(), send: vi.fn() }))
vi.mock("@/api", () => ({ api: mocks.api, send: mocks.send }))
vi.mock("@/state", () => ({ useAppState: () => ({ state: { models: { claude: ["haiku", "opus"] } } }) }))

const path = "/api/routing?dir=" + enc("/repo")
const base: RoutingView = {
  path: "/home/me/.config/tagent/routing.yaml",
  limits: { claude: 90, codex: 90 },
  when_exhausted: "block",
  harnesses: ["claude", "pi", "codex", "Claude API"],
  providers: [
    { id: "claude", name: "Claude", five: { usedPercent: 10, resetsAt: null }, week: { usedPercent: 5, resetsAt: null }, stale: false },
    { id: "codex", name: "Codex", five: { usedPercent: null, resetsAt: null }, week: { usedPercent: null, resetsAt: null }, stale: true },
  ],
  tiers: [{
    name: "fast", usedBy: ["dev"], selected: "claude:haiku", reason: "",
    candidates: [
      { ref: "claude:haiku", state: "available", why: "", until: null, threshold: 90, used: 10, selected: true, spilled: false },
      { ref: "pi:openai-codex/gpt-6-luna", state: "unknown", why: "", until: null, threshold: null, used: null, selected: false, spilled: false },
    ],
  }],
  roles: [{ name: "dev", tier: "fast", maxInstances: 0 }, { name: "review", maxInstances: 1 }],
}
let current: RoutingView
const lastPut = () => mocks.send.mock.calls.at(-1)?.[2]

beforeEach(() => {
  current = structuredClone(base)
  mocks.api.mockReset().mockImplementation(async () => structuredClone(current))
  mocks.send.mockReset().mockImplementation(async () => structuredClone(current))
  vi.spyOn(window, "confirm").mockReturnValue(true)
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

test("shows the tier with the server's selection and who uses it", async () => {
  render(<RoutingPage dir="/repo" />)
  expect(await screen.findByRole("heading", { name: "fast" })).toBeTruthy()
  expect(screen.getByText("used by dev")).toBeTruthy()
  expect(within(screen.getByRole("listitem", { name: "claude:haiku" })).getByText("Running now")).toBeTruthy()
  expect(screen.getByText("Claude haiku until 90% -> pi openai-codex/gpt-6-luna -> block")).toBeTruthy()
  expect(mocks.api).toHaveBeenCalledWith(path)
})

test("reorders candidates and saves the whole config", async () => {
  render(<RoutingPage dir="/repo" />)
  fireEvent.click(await screen.findByRole("button", { name: "Move claude:haiku down" }))
  await waitFor(() => expect(mocks.send).toHaveBeenCalled())
  expect(mocks.send.mock.calls[0].slice(0, 2)).toEqual(["PUT", path])
  expect(lastPut()).toEqual({ limits: { claude: 90, codex: 90 }, when_exhausted: "block", tiers: { fast: ["pi:openai-codex/gpt-6-luna", "claude:haiku"] } })
})

test("adds a candidate to a tier", async () => {
  render(<RoutingPage dir="/repo" />)
  fireEvent.change(await screen.findByLabelText("Add candidate model"), { target: { value: "opus" } })
  fireEvent.click(screen.getByRole("button", { name: "Add candidate" }))
  await waitFor(() => expect(mocks.send).toHaveBeenCalled())
  expect(lastPut().tiers.fast).toEqual(["claude:haiku", "pi:openai-codex/gpt-6-luna", "claude:opus"])
})

test("the harness pickers offer the served list, configured names included", async () => {
  render(<RoutingPage dir="/repo" />)
  const picker = await screen.findByLabelText("Add candidate harness")
  expect(Array.from(picker.querySelectorAll("option")).map(o => o.textContent)).toEqual(["claude", "pi", "codex", "Claude API"])
  fireEvent.change(picker, { target: { value: "Claude API" } })
  fireEvent.change(screen.getByLabelText("Add candidate model"), { target: { value: "sonnet@85" } })
  fireEvent.click(screen.getByRole("button", { name: "Add candidate" }))
  await waitFor(() => expect(mocks.send).toHaveBeenCalled())
  expect(lastPut().tiers.fast).toEqual(["claude:haiku", "pi:openai-codex/gpt-6-luna", "Claude API:sonnet@85"])
})

test("creates a tier with its first candidate", async () => {
  render(<RoutingPage dir="/repo" />)
  fireEvent.change(await screen.findByLabelText("Tier name"), { target: { value: "slow" } })
  fireEvent.change(screen.getByLabelText("Create tier model"), { target: { value: "opus" } })
  fireEvent.click(screen.getByRole("button", { name: "Create tier" }))
  await waitFor(() => expect(mocks.send).toHaveBeenCalled())
  expect(lastPut().tiers).toEqual({ fast: ["claude:haiku", "pi:openai-codex/gpt-6-luna"], slow: ["claude:opus"] })
})

test("deletes a tier after confirmation", async () => {
  render(<RoutingPage dir="/repo" />)
  fireEvent.click(await screen.findByRole("button", { name: "Delete tier fast" }))
  await waitFor(() => expect(mocks.send).toHaveBeenCalled())
  expect(window.confirm).toHaveBeenCalledWith(expect.stringContaining("dev"))
  expect(lastPut().tiers).toEqual({})
})

test("saves a changed threshold and when_exhausted", async () => {
  render(<RoutingPage dir="/repo" />)
  const limit = await screen.findByLabelText("Claude limit")
  fireEvent.change(limit, { target: { value: "80" } })
  fireEvent.blur(limit)
  await waitFor(() => expect(mocks.send).toHaveBeenCalledTimes(1))
  expect(lastPut().limits.claude).toBe(80)
  fireEvent.change(screen.getByLabelText("When exhausted"), { target: { value: "first" } })
  await waitFor(() => expect(mocks.send).toHaveBeenCalledTimes(2))
  expect(lastPut().when_exhausted).toBe("first")
})

test("sets a candidate's own threshold", async () => {
  render(<RoutingPage dir="/repo" />)
  const until = await screen.findByLabelText("claude:haiku spill at")
  fireEvent.change(until, { target: { value: "70" } })
  fireEvent.blur(until)
  await waitFor(() => expect(mocks.send).toHaveBeenCalledTimes(1))
  expect(lastPut().tiers.fast).toEqual(["claude:haiku@70", "pi:openai-codex/gpt-6-luna"])
})

test("clears a candidate's own threshold back to the provider limit", async () => {
  current.tiers[0].candidates[0] = { ...current.tiers[0].candidates[0], ref: "claude:haiku@70", until: 70, threshold: 70 }
  render(<RoutingPage dir="/repo" />)
  const until = await screen.findByLabelText("claude:haiku@70 spill at")
  fireEvent.change(until, { target: { value: "" } })
  fireEvent.blur(until)
  await waitFor(() => expect(mocks.send).toHaveBeenCalledTimes(1))
  expect(lastPut().tiers.fast).toEqual(["claude:haiku", "pi:openai-codex/gpt-6-luna"])
})

test("an empty threshold is not saved as 0", async () => {
  render(<RoutingPage dir="/repo" />)
  const limit = await screen.findByLabelText("Claude limit") as HTMLInputElement
  fireEvent.change(limit, { target: { value: "" } })
  fireEvent.blur(limit)
  expect(mocks.send).not.toHaveBeenCalled()
  expect(limit.value).toBe("90")
})

test("the selected marker follows the quota poll", async () => {
  render(<RoutingPage dir="/repo" />)
  await screen.findByText("Running now")
  current.tiers[0].selected = "pi:openai-codex/gpt-6-luna"
  current.tiers[0].candidates[0].selected = false
  current.tiers[0].candidates[1].selected = true
  current.tiers[0].reason = "claude:haiku 5h 93% >= 90"
  await waitFor(() => expect(screen.getByText("Skipped for quota: claude:haiku 5h 93% >= 90")).toBeTruthy(), { timeout: 4500 })
  expect(screen.getAllByText("Running now")).toHaveLength(1)
})

test("an invalid file shows the error and no controls", async () => {
  current = { ...base, error: "tier my.tier: use letters", tiers: [] }
  render(<RoutingPage dir="/repo" />)
  expect(await screen.findByRole("alert")).toBeTruthy()
  expect(screen.getByText(/does not load/).textContent).toContain("tier my.tier")
  expect(screen.queryByRole("button", { name: "Create tier" })).toBeNull()
  expect(screen.queryByLabelText("When exhausted")).toBeNull()
})

test("drag and drop reorders candidates", async () => {
  render(<RoutingPage dir="/repo" />)
  const row = await screen.findByRole("listitem", { name: "pi:openai-codex/gpt-6-luna" })
  fireEvent.dragStart(screen.getByLabelText("Drag claude:haiku"))
  fireEvent.dragOver(row)
  fireEvent.drop(row)
  await waitFor(() => expect(mocks.send).toHaveBeenCalled())
  expect(lastPut().tiers.fast).toEqual(["pi:openai-codex/gpt-6-luna", "claude:haiku"])
})

test("a drop without a drag starts changes nothing", async () => {
  render(<RoutingPage dir="/repo" />)
  fireEvent.drop(await screen.findByRole("listitem", { name: "pi:openai-codex/gpt-6-luna" }))
  expect(mocks.send).not.toHaveBeenCalled()
})

test("a spilled candidate shows when it resets, and the flow line keeps its threshold", async () => {
  current.tiers[0].candidates[0] = { ...current.tiers[0].candidates[0], selected: false, spilled: true, used: 95, state: "exhausted" }
  current.tiers[0].candidates[1].selected = true
  current.providers[0].five = { usedPercent: 95, resetsAt: "2026-11-27T10:00:00Z" }
  render(<RoutingPage dir="/repo" />)
  const row = await screen.findByRole("listitem", { name: "claude:haiku" })
  expect(within(row).getByText(/^Spilled - resets /)).toBeTruthy()
  expect(within(screen.getByRole("listitem", { name: "pi:openai-codex/gpt-6-luna" })).getByText("Running now")).toBeTruthy()
  expect(within(screen.getByRole("listitem", { name: "pi:openai-codex/gpt-6-luna" })).getByText("no quota")).toBeTruthy()
})

test("the defaults section holds when_exhausted and the provider limits", async () => {
  render(<RoutingPage dir="/repo" />)
  await screen.findByRole("heading", { name: "fast" })
  expect(screen.getByText("Defaults")).toBeTruthy()
  mocks.send.mockImplementation(async (_m: string, _p: string, body: { when_exhausted: string }) => { current.when_exhausted = body.when_exhausted; return structuredClone(current) })
  fireEvent.change(screen.getByLabelText("When exhausted"), { target: { value: "first" } })
  await waitFor(() => expect(mocks.send).toHaveBeenCalledTimes(1))
  expect(lastPut().when_exhausted).toBe("first")
  expect(await screen.findByText("Claude haiku until 90% -> pi openai-codex/gpt-6-luna -> run first")).toBeTruthy()
})

test("edits a role's parallel agents limit and saves only a whole number", async () => {
  render(<RoutingPage dir="/repo" />)
  const dev = await screen.findByLabelText("dev parallel agents")
  expect((screen.getByLabelText("review parallel agents") as HTMLInputElement).value).toBe("1")
  fireEvent.change(dev, { target: { value: "2" } })
  fireEvent.blur(dev)
  await waitFor(() => expect(mocks.send).toHaveBeenCalledWith("PUT", "/api/board/config/role-limit?dir=" + enc("/repo"), { role: "dev", max: 2 }))

  mocks.send.mockClear()
  fireEvent.change(screen.getByLabelText("review parallel agents"), { target: { value: "1.5" } })
  fireEvent.blur(screen.getByLabelText("review parallel agents"))
  expect(await screen.findByRole("alert")).toBeTruthy()
  expect(screen.getByText("Enter a whole number, 0 or more")).toBeTruthy()
  expect(mocks.send).not.toHaveBeenCalled()
})

test("parseRef, withUntil, reorder and flow", () => {
  expect(parseRef("claude:opus@70")).toEqual({ harness: "claude", model: "opus", until: 70 })
  expect(parseRef("pi:openai-codex/gpt-6-luna")).toEqual({ harness: "pi", model: "openai-codex/gpt-6-luna", until: null })
  expect(withUntil("claude:opus@70", "")).toBe("claude:opus")
  expect(withUntil("claude:opus", "80")).toBe("claude:opus@80")
  expect(withUntil("claude:opus@70", "80")).toBe("claude:opus@80")
  expect(reorder(["a", "b", "c"], 0, 2)).toEqual(["b", "c", "a"])
  expect(reorder(["a", "b", "c"], 2, 0)).toEqual(["c", "a", "b"])
  const cs = [
    { ref: "claude:opus@70", threshold: 70 },
    { ref: "codex:gpt-6.1-sol", threshold: 90 },
    { ref: "pi:openai-codex/gpt-6-luna", threshold: null },
  ] as RoutingCandidate[]
  expect(flow(cs, "block")).toBe("Claude opus until 70% -> Codex gpt-6.1-sol until 90% -> pi openai-codex/gpt-6-luna -> block")
  expect(flow([], "first")).toBe("run first")
})
