import { afterEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { CustomPane } from "./CustomPane"

const mocks = vi.hoisted(() => ({ api: vi.fn() }))
vi.mock("@/api", () => ({ api: mocks.api, send: vi.fn(), act: (p: Promise<unknown>) => p }))
vi.mock("@/components/ExtPane", () => ({ ExtPane: () => null }))
afterEach(() => { cleanup(); vi.unstubAllGlobals(); mocks.api.mockReset() })

test("the header deletes a pane only after the user confirms, then reloads the list", async () => {
  mocks.api.mockResolvedValue({})
  const reload = vi.fn()
  vi.stubGlobal("confirm", vi.fn().mockReturnValueOnce(false).mockReturnValue(true))
  render(<CustomPane info={{ name: "office", agent: false, actions: false }} reload={reload} />)
  const button = screen.getByRole("button", { name: "Delete office" })
  fireEvent.click(button)
  expect(mocks.api).not.toHaveBeenCalled()
  fireEvent.click(button)
  expect(mocks.api).toHaveBeenCalledWith("/api/ext/office", { method: "DELETE" })
  await waitFor(() => expect(reload).toHaveBeenCalled())
})
