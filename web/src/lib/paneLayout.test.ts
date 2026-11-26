import { readFileSync } from "node:fs"
import path from "node:path"
import { beforeEach, expect, test, vi } from "vitest"
import { defaultPanes, loadPanes, mergeBoxes, packBoxes, placeBoxes, savePanes, type PaneBox } from "./paneLayout"

// The same cases run in internal/sidebar/panegrid_test.go.
const cases: {
  name: string; minH?: number; saved: PaneBox[]; defaults?: PaneBox[]; moved?: string; pack?: Record<string, number>; want: PaneBox[]
}[] = JSON.parse(readFileSync(path.resolve(import.meta.dirname, "../../../testdata/pane-layout.json"), "utf8"))

test.each(cases)("fixture: $name", c => {
  const got = c.pack ? packBoxes(c.saved, c.pack) : mergeBoxes(c.saved, c.defaults!, c.minH!, c.moved!)
  expect(got).toEqual(c.want)
})

test("a saved three-box layout gains workflows in the visible rows, and the saved boxes do not move", () => {
  const saved = defaultPanes(24).filter(b => b.id !== "workflows")
  const out = mergeBoxes(saved, defaultPanes(24), 4, "")
  expect(out.filter(b => b.id !== "workflows")).toEqual(saved)
  expect(out.find(b => b.id === "workflows")).toMatchObject({ x: 0, y: 16, w: 3, h: 8 })
})

test("placeBoxes does not change its input", () => {
  const boxes = [{ id: "a", x: 0, y: 0, w: 6, h: 4 }, { id: "b", x: 3, y: 0, w: 6, h: 4 }]
  const copy = structuredClone(boxes)
  placeBoxes(boxes, "a")
  expect(boxes).toEqual(copy)
})

beforeEach(() => localStorage.clear())

test("a saved layout round-trips and undefined removes it", () => {
  const boxes = defaultPanes(30)
  savePanes(boxes)
  expect(loadPanes()).toEqual(boxes)
  savePanes(undefined)
  expect(loadPanes()).toBeUndefined()
})

test.each([
  "{bad",
  '{"v":2,"boxes":[]}',
  '{"v":1,"boxes":{}}',
  '{"v":1,"boxes":[{"id":"usage","x":0,"y":0,"w":3}]}',
  '{"v":1,"boxes":[{"x":0,"y":0,"w":3,"h":3}]}',
  '{"v":1,"boxes":[{"id":"usage","x":0.5,"y":0,"w":3,"h":3}]}',
  "5",
])("corrupt value %s gives no layout", raw => {
  localStorage.setItem("tagent.panes", raw)
  expect(loadPanes()).toBeUndefined()
})

test("throwing storage is ignored", () => {
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("quota") })
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("denied") })
  expect(() => savePanes(defaultPanes(24))).not.toThrow()
  expect(loadPanes()).toBeUndefined()
  vi.restoreAllMocks()
})
