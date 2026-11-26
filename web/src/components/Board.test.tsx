import { expect, test, vi } from "vitest"
import { fireEvent, render, screen, within } from "@testing-library/react"
import { Board } from "./Board"
import type { Project } from "@/types"

// Some modules read matchMedia at import time; jsdom has none.
vi.hoisted(() => {
  window.matchMedia = ((q: string) => ({ matches: false, media: q, addEventListener: () => {}, removeEventListener: () => {} })) as unknown as typeof window.matchMedia
})

test("a project with no tasks gets one empty state, not four empty columns", () => {
  render(<Board dir="/p/tagent" proj={{ tasks: [], statuses: [] } as unknown as Project} reload={() => {}} onSelect={() => {}} />)
  expect(screen.getByText("No tasks in tagent.")).toBeTruthy()
  expect(screen.queryAllByRole("section", { name: /column$/ })).toHaveLength(0)
})

test("the closed lane is collapsed until shown, and purge asks twice", () => {
  const task = { id: "7", title: "Dropped idea", text: "Dropped idea", status: "closed", needs: [] }
  render(<Board dir="/p/tagent" proj={{ tasks: [task], statuses: [{ id: "closed" }] } as unknown as Project} reload={() => {}} onSelect={() => {}} />)
  const lane = screen.getByLabelText("closed column")
  expect(within(lane).queryByText("Dropped idea")).toBeNull()
  fireEvent.click(within(lane).getByRole("button", { name: "Show" }))
  expect(within(lane).getByText("Dropped idea")).toBeTruthy()
  fireEvent.click(within(lane).getByRole("button", { name: "Delete closed" }))
  expect(within(lane).getByRole("button", { name: /Delete 1 and discard their worktrees and branches/ })).toBeTruthy()
})
