import { expect, test } from "vitest"
import { render, screen } from "@testing-library/react"
import { CustomPage } from "./CustomPage"

const page = (names: string[], name?: string) => <CustomPage exts={names.map(n => ({ name: n, agent: false, actions: false }))} name={name} reload={() => {}} />

test("a pane is an iframe sandboxed without allow-same-origin, loaded from /ext/<name>.html", () => {
  render(page(["hello", "two"], "two"))
  const frame = screen.getByTitle("two")
  expect(frame.getAttribute("sandbox")).toBe("allow-scripts")
  expect(frame.getAttribute("src")).toBe("/ext/two.html")
  expect(screen.getByRole("link", { name: "hello" }).getAttribute("href")).toBe("#/custom/hello")
})

test("no name shows the first pane; an unknown name or no panes shows the hint", () => {
  const { unmount } = render(page(["hello"]))
  expect(screen.getByTitle("hello")).toBeTruthy()
  unmount()
  const again = render(page(["hello"], "nope"))
  expect(screen.queryByTitle("hello")).toBeNull()
  expect(screen.getByText(/No custom pane/)).toBeTruthy()
  again.unmount()
  render(page([]))
  expect(screen.getByText(/No custom pane/)).toBeTruthy()
})
