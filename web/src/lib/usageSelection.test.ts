import { beforeEach, expect, test } from "vitest"
import { loadHidden, saveHidden } from "./usageSelection"

beforeEach(() => localStorage.clear())

test("hidden rows round-trip; nothing hidden removes the entry", () => {
  expect(loadHidden()).toEqual([])
  saveHidden(["provider:Codex"])
  expect(loadHidden()).toEqual(["provider:Codex"])
  saveHidden([])
  expect(localStorage.getItem("tagent.usage")).toBeNull()
})

test.each(["{bad", "5", '{"v":2,"hidden":["a"]}', '{"v":1,"hidden":"a"}'])("corrupt value %s hides nothing", raw => {
  localStorage.setItem("tagent.usage", raw)
  expect(loadHidden()).toEqual([])
})
