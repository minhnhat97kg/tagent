import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { TasksPanel } from "./TasksPanel"
import type { Project } from "@/types"

const mocks = vi.hoisted(() => ({ api: vi.fn() }))
vi.mock("@/api", () => ({ api: mocks.api, post: vi.fn(), act: (p: Promise<unknown>) => p.catch(() => {}) }))

const proj = {
  statuses: [{ id: "todo", role: "", run: "" }, { id: "build", role: "builder", run: "" }],
  tasks: [{ id: "1", title: "Alpha", text: "Alpha\nThe ticket's text", status: "todo", assignees: null, created: "2026-11-23T09:00:00Z", updated: "2026-11-23T10:00:00Z", comments: [] }],
  roles: ["builder"],
  workflows: [],
  dir: "/p",
} as unknown as Project

beforeEach(() => {
  mocks.api.mockReset().mockResolvedValue(undefined)
  vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, json: async () => [] })))
  globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver
  window.matchMedia = ((q: string) => ({ matches: true, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
})
afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

test("list: a row asks the caller to open its ticket", () => {
  const onOpen = vi.fn()
  render(<TasksPanel proj={proj} dir="/p" reload={() => {}} onOpen={onOpen} />)
  const row = screen.getByRole("button", { name: "#1 todo: Alpha" })
  fireEvent.click(row)
  expect(onOpen).toHaveBeenCalledWith("1", row)
})
