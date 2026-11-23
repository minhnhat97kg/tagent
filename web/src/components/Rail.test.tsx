import { afterEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { RailStrip } from "./Rail"

afterEach(cleanup)

test("an icon reopens the rail", () => {
  const onExpand = vi.fn()
  render(<RailStrip tasks onExpand={onExpand} />)
  fireEvent.click(screen.getByRole("button", { name: "Show agents panel" }))
  expect(onExpand).toHaveBeenCalledTimes(1)
})

test("the tickets icon follows the panels the rail shows", () => {
  render(<RailStrip tasks={false} onExpand={vi.fn()} />)
  expect(screen.getByRole("button", { name: "Show usage panel" })).toBeTruthy()
  expect(screen.queryByRole("button", { name: "Show tickets panel" })).toBeNull()
})
