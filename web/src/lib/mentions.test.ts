import { expect, test } from "vitest"
import { applyMention, filterMentions, mentionItems, mentionQuery } from "./mentions"
import type { Agent, Task, Workflow } from "@/types"

const agents = [{ pane: "%1", name: "planner-26", agent: "claude", status: "idle" }, { pane: "%2", name: "me", agent: "claude", status: "idle" }] as Agent[]
const tasks = [{ id: "25", title: "Agents management", text: "", status: "building" }, { id: "9", title: "old", text: "", status: "done" }] as Task[]
const workflows = [{ name: "feature", description: "plan, build", jobs: [], error: "" }, { name: "broken", description: "", jobs: [], error: "bad" }] as unknown as Workflow[]

test("lists other agents, open tickets and valid workflows", () => {
  const items = mentionItems(agents, tasks, workflows, "%2")
  expect(items.map(i => i.kind + ":" + i.insert)).toEqual(["agent:@planner-26", "task:task #25 (Agents management)", 'workflow:workflow "feature"'])
})

test("finds the word being typed and filters by trigger", () => {
  const items = mentionItems(agents, tasks, workflows)
  expect(mentionQuery("ask @pla", 8)).toEqual({ trigger: "@", start: 4, query: "pla" })
  expect(mentionQuery("email@x", 7)).toBeNull()
  expect(filterMentions(items, { trigger: "@", query: "feat" }).map(i => i.kind)).toEqual(["workflow"])
  expect(filterMentions(items, { trigger: "#", query: "" }).every(i => i.kind === "task")).toBe(true)
  expect(filterMentions(items, { trigger: "#", query: "25" })).toHaveLength(1)
})

test("applying a mention replaces only the typed word", () => {
  const [m] = mentionItems(agents, [], [], "%2")
  expect(applyMention("hey @pl there", 7, 4, m!)).toEqual({ text: "hey @planner-26  there", caret: 16 })
})
