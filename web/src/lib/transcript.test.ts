import { afterEach, expect, test, vi } from "vitest"
import { getMode, runningTool, setMode, toRows, type Item } from "./transcript"

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
