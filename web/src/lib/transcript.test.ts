import { afterEach, expect, test, vi } from "vitest"
import { clock, createdTask, getMode, runningTool, setMode, splitSystem, toRows, type Item } from "./transcript"

afterEach(() => { vi.restoreAllMocks() })

test("tool results merge into their call by callId", () => {
  const items: Item[] = [
    { id: "a.0", role: "assistant", text: "checking" },
    { id: "a.1", role: "tool", tool: { name: "Edit", label: "Edit Board.tsx", callId: "t1", input: "{}" } },
    { id: "a.2", role: "tool", tool: { name: "Bash", label: "Bash go test", callId: "t2", input: "{}" } },
    { id: "u.0", role: "tool", tool: { callId: "t1", output: "ok" } },
    { id: "x.0", role: "tool", tool: { callId: "gone", output: "orphan" } },
  ]
  const rows = toRows(items)
  expect(rows.map(r => r.kind)).toEqual(["msg", "tool", "tool"])
  expect(rows[1]).toMatchObject({ output: "ok" })
  expect((rows[2] as { output?: string }).output).toBeUndefined()
  expect(runningTool(rows)).toBe("Bash go test")
  expect(runningTool(toRows(items.slice(0, 2)))).toBe("Edit Board.tsx")
  expect(runningTool(toRows(items.slice(0, 1)))).toBeUndefined()
})

test("mode defaults to chat on phones, remembers the choice, and survives blocked storage", () => {
  localStorage.clear()
  window.matchMedia = vi.fn().mockReturnValue({ matches: true }) as unknown as typeof window.matchMedia
  expect(getMode("%1", true)).toBe("chat")
  expect(getMode("%1", false)).toBe("terminal")
  setMode("%1", "terminal")
  expect(getMode("%1", true)).toBe("terminal")
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("blocked") })
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("blocked") })
  expect(() => setMode("%1", "chat")).not.toThrow()
  expect(getMode("%1", true)).toBe("chat")
})

test("a task notification becomes one note and reminders are dropped", () => {
  const text = "<task-notification>\n<task-id>x</task-id>\n<summary>Background command \"tail -15\" completed (exit code 0)</summary>\n</task-notification>\n<system-reminder>ignore</system-reminder>"
  expect(splitSystem(text)).toEqual({ text: "", notes: ['Background command "tail -15" completed (exit code 0)'] })
})

test("splitSystem keeps the person's own words", () => {
  expect(splitSystem("hello <system-reminder>x</system-reminder>")).toEqual({ text: "hello", notes: [] })
})

test("slash command echoes become notes", () => {
  const t = "<command-name>/compact</command-name>\n<command-message>compact</command-message>\n<command-args></command-args>"
  expect(splitSystem(t)).toEqual({ text: "", notes: ["/compact"] })
  expect(splitSystem("<local-command-stdout>\x1b[2mCompacted\x1b[22m</local-command-stdout>").notes).toEqual(["Compacted"])
})

test("clock shows HH:MM today, with the date for older messages, and nothing when the time is bad", () => {
  const now = new Date("2026-11-25T12:00:00")
  expect(clock("2026-11-25T09:05:00", now)).toMatch(/^\d{2}:\d{2}$/)
  expect(clock("2026-11-24T09:05:00", now)).toMatch(/^\D+ \d{1,2} \d{2}:\d{2}$/)
  expect(clock(undefined, now)).toBe("")
  expect(clock("not a time", now)).toBe("")
})

test("createdTask finds tickets made by MCP task_add or `tagent task add`, nothing else", () => {
  const row = (name: string, input: string, output?: string) => ({ kind: "tool" as const, call: { id: "c", role: "tool" as const, tool: { name, input, label: name } }, output })
  expect(createdTask(row("mcp__tagent__task_add", '{"title":"Fix it"}', "70"))).toEqual({ id: "70", title: "Fix it" })
  expect(createdTask(row("Bash", '{"command":"./bin/tagent task add --desc x \\"T\\""}', "71\n"))).toEqual({ id: "71" })
  expect(createdTask(row("Bash", '{"command":"tagent task ls"}', "71"))).toBeUndefined()
  expect(createdTask(row("Bash", '{"command":"tagent task add T && tagent task move 72 backlog"}', "72\n72: T\nStatus: backlog"))).toEqual({ id: "72" })
  expect(createdTask(row("Bash", '{"command":"tagent task add T; tagent task ls"}', "error: x\n73"))).toBeUndefined()
  expect(createdTask(row("mcp__tagent__task_add", "{}", "error: title required"))).toBeUndefined()
  expect(createdTask(row("mcp__tagent__task_add", "{}"))).toBeUndefined()
})
