import { expect, test, vi } from "vitest"
import { render, screen } from "@testing-library/react"
import { Board } from "./Board"
import type { Project } from "@/types"

// Some modules read matchMedia at import time; jsdom has none.
vi.hoisted(() => {
  window.matchMedia = ((q: string) => ({ matches: false, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
})

test("a project with no tasks gets one empty state, not four empty columns", () => {
  render(<Board dir="/p/tagent" proj={{ tasks: [], statuses: [] } as unknown as Project} reload={() => {}} />)
  expect(screen.getByText("No tasks in tagent.")).toBeTruthy()
  expect(screen.queryAllByRole("section", { name: /column$/ })).toHaveLength(0)
})
