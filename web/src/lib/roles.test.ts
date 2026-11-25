import { describe, expect, it } from "vitest"
import { contextList, contextText, editableKey, effortLevels, modelsIn, readOnlySource, roleTemplate, usedBySummary, withContext } from "@/lib/roles"
import type { Role } from "@/types"

const role = (over: Partial<Role>): Role => ({ name: "a", harness: "pi", model: "", effort: "", source: "board", path: "", instructions: "", running: 0, panes: null, usedBy: null, ...over })

describe("roles helpers", () => {
  it("lists effort levels per harness", () => {
    expect(effortLevels("claude")).not.toContain("off")
    expect(effortLevels("codex")).toContain("none")
    expect(effortLevels("opencode")).toEqual([])
  })
  it("suggests each model once, sorted", () => {
    expect(modelsIn([role({ model: "b" }), role({ model: "a" }), role({ model: "b" }), role({})])).toEqual(["a", "b"])
  })
  it("says when nothing uses a role", () => {
    expect(usedBySummary(role({}))).toContain("not used")
    expect(usedBySummary(role({ usedBy: ["status plan"] }))).toBe("status plan")
  })
  it("compares only what the user edits", () => {
    expect(editableKey(role({ running: 2, usedBy: ["status plan"] }))).toBe(editableKey(role({})))
    expect(editableKey(role({ model: "n" }))).not.toBe(editableKey(role({})))
  })
  it("treats context edits as dirty and parses context text", () => {
    expect(editableKey(role({ mcp: ["tagent"] }))).not.toBe(editableKey(role({})))
    expect(contextList("", true)).toBeUndefined()
    expect(contextList("none", true)).toEqual([])
    expect(contextList("", false)).toEqual([])
    expect(contextList("Read, Grep,", false)).toEqual(["Read", "Grep"])
    expect(contextText([], true)).toBe("none")
    expect(contextText(undefined, true)).toBe("")
  })
  it("sends all five context keys on save, cleared ones as default or none", () => {
    const sent = withContext(role({}))
    expect(sent).toMatchObject({ skills: null, mcp: null, tools: [], allow: [], deny: [] })
    expect(withContext(role({ mcp: ["tagent"] })).mcp).toEqual(["tagent"])
  })
  it("builds a plain markdown template without frontmatter", () => {
    expect(roleTemplate("writer")).toBe("# writer\n\nDescribe how this agent works.\n")
    expect(roleTemplate("writer").startsWith("---")).toBe(false)
  })
  it("marks only legacy agents read-only", () => {
    expect(readOnlySource("global")).toBe(false)
    expect(readOnlySource("legacy")).toBe(true)
    expect(readOnlySource("repo")).toBe(false)
    expect(readOnlySource("board")).toBe(false)
  })
})
