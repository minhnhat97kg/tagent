import { beforeEach, expect, test, vi } from "vitest"
import { DEFAULT_PANES, loadPanes, savePanes } from "./paneLayout"

beforeEach(() => localStorage.clear())

test("a saved layout round-trips and undefined removes it", () => {
  const sizes = { cols: [0.3, 0.4, 0.3], left: [0.5, 0.5] }
  savePanes(sizes)
  expect(loadPanes()).toEqual(sizes)
  savePanes(undefined)
  expect(loadPanes()).toBeUndefined()
})

test.each([
  "{bad",
  "5",
  '{"v":1,"boxes":[{"id":"usage","x":0,"y":0,"w":3,"h":3}]}', // the old grid boxes: defaults
  '{"v":3,"cols":[0.22,0.43,0.35],"left":[0.65,0.35]}',
  '{"v":2,"cols":[0.5,0.5],"left":[0.65,0.35]}',
  '{"v":2,"cols":[0.22,0.43,0.35],"left":[0.65]}',
  '{"v":2,"cols":[0.9,0.9,0.9],"left":[0.65,0.35]}', // does not add up to 1
  '{"v":2,"cols":["a",0.5,0.5],"left":[0.65,0.35]}',
  '{"v":2,"cols":[0.22,0.43,0.35],"left":null}',
])("corrupt or old value %s gives no layout", raw => {
  localStorage.setItem("tagent.panes", raw)
  expect(loadPanes()).toBeUndefined()
})

test("the defaults add up to 1 in each split", () => {
  expect(DEFAULT_PANES.cols.reduce((s, v) => s + v, 0)).toBeCloseTo(1)
  expect(DEFAULT_PANES.left.reduce((s, v) => s + v, 0)).toBeCloseTo(1)
})

test("throwing storage is ignored", () => {
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("quota") })
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("denied") })
  expect(() => savePanes(DEFAULT_PANES)).not.toThrow()
  expect(loadPanes()).toBeUndefined()
  vi.restoreAllMocks()
})
