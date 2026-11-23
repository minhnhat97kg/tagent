import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { act, cleanup, renderHook } from "@testing-library/react"
import { useRailCollapsed } from "@/hooks/useRail"
import { getRailCollapsed, setRailCollapsed } from "./rail"

beforeEach(() => localStorage.clear())
afterEach(cleanup)

test("the rail starts expanded", () => {
  expect(getRailCollapsed()).toBe(false)
})

test("remembers the collapsed rail per device", () => {
  setRailCollapsed(true)
  expect(localStorage.getItem("tagent.rail")).toBe("collapsed")
  expect(getRailCollapsed()).toBe(true)
  setRailCollapsed(false)
  expect(getRailCollapsed()).toBe(false)
})

test("blocked storage leaves the rail usable", () => {
  const get = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("blocked")
  })
  const set = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("blocked")
  })
  expect(getRailCollapsed()).toBe(false)
  expect(() => setRailCollapsed(true)).not.toThrow()
  get.mockRestore()
  set.mockRestore()
})

test("the hook toggles and stores the choice", () => {
  const { result } = renderHook(() => useRailCollapsed())
  expect(result.current[0]).toBe(false)
  act(() => result.current[1](true))
  expect(result.current[0]).toBe(true)
  expect(localStorage.getItem("tagent.rail")).toBe("collapsed")
})
