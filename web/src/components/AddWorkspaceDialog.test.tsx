import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen } from "@testing-library/react"
import { AddWorkspaceDialog } from "./AddWorkspaceDialog"
import { WorkspaceSettings } from "./SettingsPage"
import { ProjectSelect } from "./ProjectSelect"
import { api, post } from "@/api"

vi.mock("@/api", () => ({ api: vi.fn(), post: vi.fn(), act: vi.fn() }))
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
vi.mock("@/state", () => ({ useAppState: () => ({ state: { projects: ["/home/u/a"] } }) }))

const listings: Record<string, { path: string; parent: string; dirs: string[] }> = {
  "/home/u": { path: "/home/u", parent: "/home", dirs: ["code"] },
  "/home/u/code": { path: "/home/u/code", parent: "/home/u", dirs: ["repo"] },
}
beforeEach(() => {
  Element.prototype.scrollIntoView = vi.fn() // jsdom has none; Radix Select calls it
  vi.mocked(api).mockReset().mockImplementation(async (p: string) => {
    const q = decodeURIComponent(p.slice(p.indexOf("=") + 1))
    return listings[q || "/home/u"] // no path means $HOME
  })
  vi.mocked(post).mockReset().mockResolvedValue({ dir: "/home/u/code" })
})
afterEach(() => cleanup())

test("picker descends, goes up, and adds the chosen folder", async () => {
  const onAdded = vi.fn()
  const onOpenChange = vi.fn()
  render(<AddWorkspaceDialog open onOpenChange={onOpenChange} onAdded={onAdded} />)
  fireEvent.click(await screen.findByRole("button", { name: "code" }))
  expect((await screen.findByLabelText("Folder") as HTMLInputElement).value).toBe("/home/u/code")
  fireEvent.click(screen.getByRole("button", { name: ".." }))
  expect((await screen.findByLabelText("Folder") as HTMLInputElement).value).toBe("/home/u")
  fireEvent.click(screen.getByRole("button", { name: "code" }))
  await screen.findByRole("button", { name: "repo" })
  fireEvent.click(screen.getByRole("button", { name: "Select this folder" }))
  await vi.waitFor(() => expect(onAdded).toHaveBeenCalledWith("/home/u/code"))
  expect(post).toHaveBeenCalledWith("/api/workspaces", { Dir: "/home/u/code" })
  expect(onOpenChange).toHaveBeenCalledWith(false)
})

test("Settings lists workspaces, removes one and opens the picker", async () => {
  const onAddWorkspace = vi.fn()
  render(<WorkspaceSettings onAddWorkspace={onAddWorkspace} />)
  fireEvent.click(await screen.findByRole("button", { name: "Remove a" }))
  await vi.waitFor(() => expect(api).toHaveBeenCalledWith("/api/workspaces?dir=%2Fhome%2Fu%2Fa", { method: "DELETE" }))
  fireEvent.click(screen.getByRole("button", { name: /Add workspace/ }))
  expect(onAddWorkspace).toHaveBeenCalled()
})

test("header dropdown offers Add workspace and opens the picker", async () => {
  const onAdd = vi.fn()
  render(<ProjectSelect value="" projects={[]} onChange={() => {}} onAdd={onAdd} />)
  const trigger = screen.getByRole("combobox")
  fireEvent.keyDown(trigger, { key: "Enter" })
  fireEvent.click(await screen.findByRole("option", { name: "Add workspace…" }))
  expect(onAdd).toHaveBeenCalled()
})
