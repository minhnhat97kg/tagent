import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import { BoardConfigPage } from "./BoardConfigPage"

const mocks = vi.hoisted(() => ({ api: vi.fn(), send: vi.fn() }))
vi.mock("@/api", () => ({ api: mocks.api, send: mocks.send }))
// CodeMirror is not the subject here; the textarea keeps the draft edits observable.
vi.mock("@/components/WorkflowYamlEditor", () => ({
  default: ({ value, onChange }: { value: string; onChange: (v: string) => void }) =>
    <textarea aria-label="YAML editor" value={value} onChange={e => onChange(e.target.value)} />,
}))

const summary = {
  defaultWorkflow: "feature",
  roles: [{ name: "planner", agent: "pi", model: "gpt", effort: "high" }],
  triggers: [{ on: "mention", when: "to=*", agent: "{{event.name}}" }],
  maxFiresPerHour: 20,
}
const workflows = [{ name: "feature", global: false }, { name: "release", global: true }]
const initial = {
  dir: "/repo",
  path: "/repo/.tasks/board.yaml",
  yaml: "default_workflow: feature\n",
  default: "default_workflow: feature\n",
  exists: true,
  summary,
  workflows,
}
const editor = () => screen.getByLabelText("YAML editor") as HTMLTextAreaElement
const saveButton = () => screen.getByRole("button", { name: /save/i }) as HTMLButtonElement

beforeEach(() => {
  mocks.api.mockReset().mockResolvedValue(initial)
  mocks.send.mockReset().mockResolvedValue({ summary, yaml: initial.yaml, workflows })
})
afterEach(cleanup)

test("shows the same summary as the TUI and previews the draft", async () => {
  render(<BoardConfigPage dir="/repo" />)
  await waitFor(() => expect(editor().value).toBe(initial.yaml))
  expect(screen.getByRole("heading", { name: /default workflow/i })).toBeTruthy()
  expect(screen.getAllByText("feature").length).toBeGreaterThan(0)
  expect(screen.getByText(/flows live in \.tasks\/workflows/i)).toBeTruthy()
  expect(screen.getByText("planner")).toBeTruthy()
  expect(screen.getByText(/pi gpt · effort high/)).toBeTruthy()
  expect(screen.getByText("mention")).toBeTruthy()
  expect(screen.getByText(/agent \{\{event.name\}\}/)).toBeTruthy()
  expect(saveButton().disabled).toBe(true)

  fireEvent.change(editor(), { target: { value: "default_workflow: release\n" } })
  await waitFor(() => expect(mocks.send).toHaveBeenLastCalledWith("PUT", expect.stringContaining("/api/board/config?dir=%2Frepo"), { yaml: "default_workflow: release\n", validate: true }))
  await waitFor(() => expect(saveButton().disabled).toBe(false))
})

test("the default workflow select saves through the endpoint", async () => {
  mocks.send.mockResolvedValue({ summary: { ...summary, defaultWorkflow: "release" }, yaml: "default_workflow: release\n", workflows })
  render(<BoardConfigPage dir="/repo" />)
  const select = (await screen.findByLabelText("Default workflow")) as HTMLSelectElement
  await waitFor(() => expect(select.value).toBe("feature"))
  expect(screen.getByText("release (global)")).toBeTruthy()
  fireEvent.change(select, { target: { value: "release" } })
  await waitFor(() => expect(mocks.send).toHaveBeenCalledWith("PUT", expect.stringContaining("/api/board/config/default-workflow?dir=%2Frepo"), { name: "release" }))
  await waitFor(() => expect(editor().value).toBe("default_workflow: release\n"))
})

test("a validation error names its line and blocks Save", async () => {
  mocks.send.mockRejectedValue(new Error(JSON.stringify({ error: 'line 2: invalid next "ghost"', line: 2 })))
  render(<BoardConfigPage dir="/repo" />)
  const alert = await screen.findByRole("alert")
  expect(alert.textContent).toContain('Line 2: invalid next "ghost"')
  expect(saveButton().disabled).toBe(true)
})

test("Restore defaults fills the editor and Revert returns to the saved text", async () => {
  render(<BoardConfigPage dir="/repo" />)
  await waitFor(() => expect(editor().value).toBe(initial.yaml))
  fireEvent.click(screen.getByRole("button", { name: /restore defaults/i }))
  expect(editor().value).toBe(initial.default)
  fireEvent.click(screen.getByRole("button", { name: /revert/i }))
  expect(editor().value).toBe(initial.yaml)
})
