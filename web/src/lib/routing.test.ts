import { describe, expect, it } from "vitest"
import { parseRef, withEffort, withUntil } from "@/lib/routing"

describe("candidate refs", () => {
  it("reads harness:model@N#effort, and old refs without an effort", () => {
    expect(parseRef("Claude API:haiku@85#high")).toEqual({ harness: "Claude API", model: "haiku", until: 85, effort: "high" })
    expect(parseRef("pi:openai-codex/gpt-6-luna")).toEqual({ harness: "pi", model: "openai-codex/gpt-6-luna", until: null, effort: "" })
  })
  it("sets and clears the effort without losing the threshold", () => {
    expect(withEffort("claude:haiku@85", "high")).toBe("claude:haiku@85#high")
    expect(withEffort("claude:haiku@85#high", "")).toBe("claude:haiku@85")
    expect(withEffort("claude:haiku#low", "max")).toBe("claude:haiku#max")
  })
  it("keeps the effort when the threshold changes", () => {
    expect(withUntil("claude:haiku#high", "70")).toBe("claude:haiku@70#high")
    expect(withUntil("claude:haiku@70#high", "")).toBe("claude:haiku#high")
  })
})
