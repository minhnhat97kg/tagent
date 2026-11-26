import { expect, test } from "vitest"
import { render, screen } from "@testing-library/react"
import { CustomPage } from "./CustomPage"

test("a pane is an iframe sandboxed without allow-same-origin, loaded from /ext/<name>.html", () => {
  render(<CustomPage names={["hello", "two"]} name="two" />)
  const frame = screen.getByTitle("two")
  expect(frame.getAttribute("sandbox")).toBe("allow-scripts")
  expect(frame.getAttribute("src")).toBe("/ext/two.html")
  expect(screen.getByRole("link", { name: "hello" }).getAttribute("href")).toBe("#/custom/hello")
})

test("no name shows the first pane; an unknown name or no panes shows the hint", () => {
  const { unmount } = render(<CustomPage names={["hello"]} />)
  expect(screen.getByTitle("hello")).toBeTruthy()
  unmount()
  const again = render(<CustomPage names={["hello"]} name="nope" />)
  expect(screen.queryByTitle("hello")).toBeNull()
  expect(screen.getByText(/No custom pane/)).toBeTruthy()
  again.unmount()
  render(<CustomPage names={[]} />)
  expect(screen.getByText(/No custom pane/)).toBeTruthy()
})
