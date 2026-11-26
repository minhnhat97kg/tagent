import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { RoutingPage } from "./RoutingPage"
import { enc } from "@/hooks/useRoute"
import type { RoutingView } from "@/types"

const mocks = vi.hoisted(() => ({ api: vi.fn(), send: vi.fn() }))
vi.mock("@/api", () => ({ api: mocks.api, send: mocks.send }))
vi.mock("@/state", () => ({ useAppState: () => ({ state: { models: { claude: ["haiku", "opus"] } } }) }))

const path = "/api/routing?dir=" + enc("/repo")
const base: RoutingView = {
  path: "/home/me/.config/tagent/routing.yaml",
  limits: { claude: 90, codex: 90 },
  when_exhausted: "block",
  providers: [
    { id: "claude", name: "Claude", five: { usedPercent: 10, resetsAt: null }, week: { usedPercent: 5, resetsAt: null }, stale: false },
    { id: "codex", name: "Codex", five: { usedPercent: null, resetsAt: null }, week: { usedPercent: null, resetsAt: null }, stale: true },
  ],
  tiers: [{
    name: "fast", usedBy: ["dev"], selected: "claude:haiku", reason: "",
    candidates: [
      { ref: "claude:haiku", state: "available", why: "", until: null, threshold: 90, used: 10, selected: true, spilled: false },
      { ref: "pi:openai-codex/gpt-6-luna", state: "unknown", why: "", until: null, threshold: 90, used: null, selected: false, spilled: false },
    ],
  }],
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
  expect(screen.getByText("selected")).toBeTruthy()
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
  const until = await screen.findByLabelText("claude:haiku threshold")
  fireEvent.change(until, { target: { value: "70" } })
  fireEvent.blur(until)
  await waitFor(() => expect(mocks.send).toHaveBeenCalledTimes(1))
  expect(lastPut().tiers.fast).toEqual(["claude:haiku@70", "pi:openai-codex/gpt-6-luna"])
})

test("clears a candidate's own threshold back to the provider limit", async () => {
  current.tiers[0].candidates[0] = { ...current.tiers[0].candidates[0], ref: "claude:haiku@70", until: 70, threshold: 70 }
  render(<RoutingPage dir="/repo" />)
  const until = await screen.findByLabelText("claude:haiku@70 threshold")
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
  await screen.findByText("selected")
  current.tiers[0].selected = "pi:openai-codex/gpt-6-luna"
  current.tiers[0].candidates[0].selected = false
  current.tiers[0].candidates[1].selected = true
  current.tiers[0].reason = "claude:haiku 5h 93% >= 90"
  await waitFor(() => expect(screen.getByText("Skipped for quota: claude:haiku 5h 93% >= 90")).toBeTruthy(), { timeout: 4500 })
  expect(screen.getAllByText("selected")).toHaveLength(1)
})

test("an invalid file shows the error and no controls", async () => {
  current = { ...base, error: "tier my.tier: use letters", tiers: [] }
  render(<RoutingPage dir="/repo" />)
  expect(await screen.findByRole("alert")).toBeTruthy()
  expect(screen.getByText(/does not load/).textContent).toContain("tier my.tier")
  expect(screen.queryByRole("button", { name: "Create tier" })).toBeNull()
  expect(screen.queryByLabelText("When exhausted")).toBeNull()
})
