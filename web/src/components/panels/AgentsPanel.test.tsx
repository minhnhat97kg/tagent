import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { AgentsPanel } from "./AgentsPanel"
import type { Agent } from "@/types"

afterEach(() => { cleanup(); localStorage.clear(); vi.unstubAllGlobals() })

const agent = (pane: string, name: string, project: string, status = "idle") =>
  ({ pane, name, project, status, agent: "claude", path: "/" + project, session: "", window: "", prompt: "" }) as Agent

test("project and task groups fold, say what they hide, and stay folded", () => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener: () => {}, removeEventListener: () => {} }))
  vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} })
  const list = [agent("%1", "worker-7", "one", "waiting"), agent("%2", "other", "two")]
  const panel = () => render(<AgentsPanel ordered={list} byTask onToggleTask={() => {}} onSelect={() => {}} />)
  panel()
  fireEvent.click(screen.getByRole("button", { name: "Task #7" }))
  expect(screen.queryByText("worker-7")).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: /^one/ }))
  expect(screen.queryByRole("button", { name: /^Task #7/ })).toBeNull()
  expect(screen.getByRole("button", { name: /^one.*1 waiting/, expanded: false })).toBeTruthy()
  expect(screen.getByText("other")).toBeTruthy()
  cleanup()
  panel()
  fireEvent.click(screen.getByRole("button", { name: /^one/ }))
  expect(screen.getByRole("button", { name: /^Task #7/, expanded: false })).toBeTruthy()
})
