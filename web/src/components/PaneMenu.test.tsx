import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { api, post, send } from "@/api"
import { CustomPane } from "./CustomPane"
import { PaneMenu } from "./PaneMenu"
import { PANE_TEMPLATE } from "@/lib/ext"

vi.mock("@/api", () => ({ api: vi.fn(), post: vi.fn(), send: vi.fn(), act: (p: Promise<unknown>) => p }))

beforeEach(() => {
  vi.mocked(api).mockReset().mockResolvedValue({})
  vi.mocked(post).mockReset().mockResolvedValue({})
  vi.mocked(send).mockReset().mockResolvedValue({})
})
afterEach(() => { cleanup(); vi.restoreAllMocks() })

const open = () => fireEvent.keyDown(screen.getByRole("button", { name: "Add pane" }), { key: "Enter" })
const exts = [{ name: "notes", agent: false, actions: false }, { name: "bot", agent: true, actions: false }]

test("the menu lists the built-in and custom panes not in the layout, and adds the one chosen", async () => {
  const onAdd = vi.fn()
  render(<PaneMenu shown={["chat", "ext:notes"]} exts={exts} onAdd={onAdd} onChanged={() => Promise.resolve()} />)
  open()
  expect((await screen.findAllByRole("menuitem")).map(i => i.textContent)).toEqual(["Agents", "Usage", "Tasks", "bot · agent", "New custom pane…", "Delete notes…", "Delete bot…"])
  fireEvent.click(screen.getByRole("menuitem", { name: "Usage" }))
  expect(onAdd).toHaveBeenCalledWith("usage")
})

test("with every pane shown the menu says so", async () => {
  render(<PaneMenu shown={["agents", "usage", "chat", "tasks"]} exts={[]} onAdd={vi.fn()} onChanged={() => Promise.resolve()} />)
  open()
  expect(await screen.findByText("All panes are shown")).toBeTruthy()
})

test("New custom pane starts from a working template, creates the pane, then adds it", async () => {
  const onAdd = vi.fn()
  const onChanged = vi.fn().mockResolvedValue(undefined)
  render(<PaneMenu shown={[]} exts={[]} onAdd={onAdd} onChanged={onChanged} />)
  open()
  fireEvent.click(await screen.findByRole("menuitem", { name: "New custom pane…" }))
  const html = screen.getByLabelText("Pane HTML") as HTMLTextAreaElement
  expect(html.value).toBe(PANE_TEMPLATE)
  expect(html.value).toContain("/ext/sdk.js")
  const create = screen.getByRole("button", { name: "Create" }) as HTMLButtonElement
  expect(create.disabled).toBe(true) // no name
  fireEvent.change(screen.getByLabelText("Pane name"), { target: { value: "bad name" } })
  expect(create.disabled).toBe(true)
  fireEvent.change(screen.getByLabelText("Pane name"), { target: { value: "my-pane" } })
  fireEvent.click(create)
  await waitFor(() => expect(onAdd).toHaveBeenCalledWith("ext:my-pane"))
  expect(post).toHaveBeenCalledWith("/api/ext", { name: "my-pane", html: PANE_TEMPLATE })
  expect(onChanged).toHaveBeenCalled()
})

test("delete asks first", async () => {
  const onChanged = vi.fn().mockResolvedValue(undefined)
  const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true)
  render(<PaneMenu shown={[]} exts={exts} onAdd={vi.fn()} onChanged={onChanged} />)
  open()
  fireEvent.click(await screen.findByRole("menuitem", { name: "Delete notes…" }))
  expect(confirm).toHaveBeenCalledTimes(1)
  expect(api).not.toHaveBeenCalled()
  open()
  fireEvent.click(await screen.findByRole("menuitem", { name: "Delete notes…" }))
  await waitFor(() => expect(onChanged).toHaveBeenCalled())
  expect(api).toHaveBeenCalledWith("/api/ext/notes", { method: "DELETE" })
})

test("an agent's pane shows its badge; Allow actions asks, then saves the grant", async () => {
  const reload = vi.fn()
  const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true)
  const { rerender } = render(<CustomPane info={{ name: "bot", agent: true, actions: false }} reload={reload} />)
  expect(screen.getByText("agent · read-only")).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: "Allow actions" }))
  expect(send).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: "Allow actions" }))
  await waitFor(() => expect(reload).toHaveBeenCalled())
  expect(send).toHaveBeenCalledWith("PUT", "/api/ext/bot/grant", { allow: true })
  expect(confirm).toHaveBeenCalledTimes(2)
  rerender(<CustomPane info={{ name: "bot", agent: true, actions: true }} reload={reload} />)
  expect(screen.getByText("agent · actions allowed")).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: "Revoke" }))
  expect(send).toHaveBeenLastCalledWith("PUT", "/api/ext/bot/grant", { allow: false })
  expect(confirm).toHaveBeenCalledTimes(2) // revoking needs no question
})

test("the user's own pane has no badge and no grant button", () => {
  render(<CustomPane info={{ name: "mine", agent: false, actions: false }} reload={vi.fn()} />)
  expect(screen.queryByText(/agent ·/)).toBeNull()
  expect(screen.queryByRole("button", { name: /actions|Revoke/ })).toBeNull()
  expect(screen.getByTitle("mine").getAttribute("sandbox")).toBe("allow-scripts")
})
