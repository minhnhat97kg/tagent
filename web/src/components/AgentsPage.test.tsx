import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { AgentsPage } from "./AgentsPage"
import { enc } from "@/hooks/useRoute"

const mocks = vi.hoisted(() => ({ api: vi.fn(), send: vi.fn(), models: {} as Record<string, string[]> }))
vi.mock("@/api", () => ({ api: mocks.api, send: mocks.send }))
vi.mock("@/state", () => ({ useAppState: () => ({ state: { models: mocks.models } }) }))

const dev = { name: "dev", harness: "claude", model: "opus", effort: "medium", source: "repo", path: ".tasks/agents/dev.md", instructions: "# dev", running: 0, panes: [], usedBy: [] }
const lastBody = () => mocks.send.mock.calls.at(-1)?.[2]

beforeEach(() => {
  mocks.models = { claude: ["haiku", "opus"] }
  mocks.api.mockReset().mockImplementation(async (p: string) =>
    p.startsWith("/api/roles") ? [dev] : { path: "", limits: {}, when_exhausted: "block", providers: [], tiers: [{ name: "fast", usedBy: [], candidates: [], selected: "", reason: "" }] })
  mocks.send.mockReset().mockResolvedValue({})
})
afterEach(cleanup)

async function openDev() {
  fireEvent.click(await screen.findByRole("button", { name: /^dev/ }))
}

test("picking a tier hides the harness, disables context and offers every effort level", async () => {
  render(<AgentsPage dir="/repo" />)
  await openDev()
  fireEvent.change(screen.getByLabelText("Model"), { target: { value: "tier:fast" } })
  expect(screen.queryByLabelText("Harness")).toBeNull()
  expect((screen.getByLabelText("Skills") as HTMLInputElement).disabled).toBe(true)
  const effort = screen.getByLabelText("Effort") as HTMLSelectElement
  const levels = [...effort.options].map(o => o.value)
  expect(levels).toEqual(expect.arrayContaining(["off", "max"]))

  fireEvent.click(screen.getByRole("button", { name: "Save" }))
  await waitFor(() => expect(mocks.send).toHaveBeenCalled())
  expect(mocks.send.mock.calls[0].slice(0, 2)).toEqual(["POST", "/api/roles?dir=" + enc("/repo")])
  expect(lastBody()).toMatchObject({ name: "dev", tier: "fast", harness: "", model: "", skills: null, mcp: null, tools: [], allow: [], deny: [] })
})

test("a tier works when the harness has no model list", async () => {
  mocks.models = {}
  render(<AgentsPage dir="/repo" />)
  await openDev()
  fireEvent.change(screen.getByLabelText("Model"), { target: { value: "tier:fast" } })
  expect(screen.queryByLabelText("Harness")).toBeNull()
  expect(screen.getByRole("option", { name: "fast" })).toBeTruthy()
})

test("switching back to a fixed model defaults the harness to claude", async () => {
  render(<AgentsPage dir="/repo" />)
  await openDev()
  fireEvent.change(screen.getByLabelText("Model"), { target: { value: "tier:fast" } })
  fireEvent.change(screen.getByLabelText("Model"), { target: { value: "haiku" } })
  expect((screen.getByLabelText("Harness") as HTMLSelectElement).value).toBe("claude")
  expect((screen.getByLabelText("Skills") as HTMLInputElement).disabled).toBe(false)
  fireEvent.click(screen.getByRole("button", { name: "Save" }))
  await waitFor(() => expect(mocks.send).toHaveBeenCalled())
  expect(lastBody()).toMatchObject({ tier: undefined, harness: "claude", model: "haiku" })
})

test("the list shows a tier role as tier <name>", async () => {
  mocks.api.mockReset().mockImplementation(async (p: string) =>
    p.startsWith("/api/roles") ? [{ ...dev, harness: "", model: "", tier: "fast" }] : { path: "", limits: {}, when_exhausted: "block", providers: [], tiers: [] })
  render(<AgentsPage dir="/repo" />)
  expect(await screen.findByText("tier fast")).toBeTruthy()
})
