import { beforeEach, expect, test, vi } from "vitest"
import { DEFAULT_LAYOUT, dock, dropAt, fits, loadPanes, normalize, paneRect, pathOf, reopen, savePanes, settle, shape, step, type Layout, type PaneId, type Split, type Zone } from "./paneLayout"

beforeEach(() => localStorage.clear())

const x = (sizes: number[], ...children: Layout[]): Split => ({ axis: "x", sizes, children })
const y = (sizes: number[], ...children: Layout[]): Split => ({ axis: "y", sizes, children })
const sum = (a: number[]) => a.reduce((s, v) => s + v, 0)
// Every pane once, no split of one, no split inside one of the same axis, fractions that add up.
const sound = (n: Layout | undefined) => {
  const panes: string[] = []
  const walk = (node: Layout, parent?: string) => {
    if (typeof node === "string") return void panes.push(node)
    expect(node.children.length).toBeGreaterThan(1)
    expect(node.axis).not.toBe(parent)
    expect(sum(node.sizes)).toBeCloseTo(1)
    node.children.forEach(c => walk(c, node.axis))
  }
  walk(n!)
  expect(panes.sort()).toEqual(["agents", "chat", "tasks", "usage"])
}
const to = (root: Layout, id: PaneId, target: PaneId, zone: Zone) => dock(root, id, pathOf(root, target)!, zone)

test("drop Usage below Chat: Chat's height is halved and Agents takes the freed room", () => {
  const next = to(DEFAULT_LAYOUT, "usage", "chat", "below") as Split
  expect(next).toEqual(x([0.22, 0.43, 0.35], "agents", y([0.5, 0.5], "chat", "usage"), "tasks"))
  sound(next)
})

test("removing the last pane of a stack normalizes: no split of one, no split inside its own axis", () => {
  const stacked = to(DEFAULT_LAYOUT, "chat", "tasks", "below") as Split
  expect(shape(stacked)).toBe("x(y(agents usage) y(tasks chat))")
  expect(stacked.sizes[0]).toBeCloseTo(0.22 / 0.57)
  // Chat leaves for the bottom of everything: the stack becomes Tasks alone in the row.
  const full = dock(stacked, "chat", [], "below")!
  expect(shape(full)).toBe("y(x(y(agents usage) tasks) chat)")
  sound(full)
  // Usage leaves too: its stack goes, and Agents joins the row directly.
  const flat = to(full, "usage", "chat", "below")!
  expect(shape(flat)).toBe("y(x(agents tasks) chat usage)")
  sound(flat)
  expect(normalize(x([0.5, 0.5], x([0.5, 0.5], "agents", "usage"), y([1], x([0.4, 0.6], "chat", "tasks")))))
    .toEqual(x([0.25, 0.25, 0.2, 0.3], "agents", "usage", "chat", "tasks"))
})

test("a full-width bottom spans everything", () => {
  const next = dock(DEFAULT_LAYOUT, "tasks", [], "below") as Split
  expect(shape(next)).toBe("y(x(y(agents usage) chat) tasks)")
  expect(next.sizes).toEqual([0.5, 0.5])
  expect(paneRect(next, "tasks", 1000, 808, 8)).toEqual({ left: 0, top: 408, width: 1000, height: 400 })
  sound(next)
})

test("Tasks spans two of three columns: under both, then the third beside them", () => {
  let next = dock(DEFAULT_LAYOUT, "tasks", [], "below")!
  next = dock(next, "usage", [], "right")!
  expect(shape(next)).toBe("x(y(x(agents chat) tasks) usage)")
  sound(next)
  const [agents, chat, usage, tasks] = (["agents", "chat", "usage", "tasks"] as const).map(id => paneRect(next, id, 1600, 800, 8))
  expect(tasks!.left).toBe(agents!.left)
  expect(tasks!.width).toBeCloseTo(chat!.left + chat!.width)
  expect(usage!.height).toBe(800)
})

test("a group's side spans exactly that group", () => {
  const three = y([0.5, 0.5], x([0.3, 0.3, 0.4], "agents", "chat", "usage"), "tasks")
  const column = x([0.5, 0.5], y([0.5, 0.5], x([0.5, 0.5], "agents", "chat"), "usage"), "tasks")
  expect(shape(dock(column, "tasks", [0], "below")!)).toBe("y(x(agents chat) usage tasks)")
  expect(dock(column, "usage", [0, 0], "below")).toBeUndefined() // already under that row
  expect(shape(dock(three, "tasks", [0], "above")!)).toBe("y(tasks x(agents chat usage))")
})

test.each([
  ["itself", "chat", "chat", "above"],
  ["above its lower neighbour", "agents", "usage", "above"],
  ["below its upper neighbour", "usage", "agents", "below"],
  ["left of the next column", "chat", "tasks", "left"],
  ["right of the one before", "tasks", "chat", "right"],
] as const)("a drop %s changes nothing", (_, id, target, zone) => {
  expect(to(DEFAULT_LAYOUT, id, target, zone)).toBeUndefined()
})

test("swap trades places and keeps the sizes of the places", () => {
  expect(to(DEFAULT_LAYOUT, "chat", "agents", "swap")).toEqual(x([0.22, 0.43, 0.35], y([0.65, 0.35], "chat", "usage"), "agents", "tasks"))
})

// A drop on a 400x600 pane with 32px strips, anything allowed.
const at = (root: Layout, id: PaneId, target: PaneId, px: number, py: number, ok: (n: Layout) => boolean = () => true) => {
  const d = dropAt(root, id, target, px, py, 400, 600, 32, ok)
  return d && `${d.zone} ${JSON.stringify(d.path)} ${shape(d.next)}`
}

test("from a pane's side inwards: the whole page, then the pane's own quarter, then the swap in the middle", () => {
  // Chat is a column of the default row: along the bottom, the page; further in, below Chat.
  expect(at(DEFAULT_LAYOUT, "tasks", "chat", 200, 590)).toBe("below [] y(x(y(agents usage) chat) tasks)")
  expect(at(DEFAULT_LAYOUT, "tasks", "chat", 200, 550)).toBe("below [1] x(y(agents usage) y(chat tasks))")
  expect(at(DEFAULT_LAYOUT, "tasks", "chat", 200, 300)).toBe("swap [1] x(y(agents usage) tasks chat)")
  // Over itself, only the larger places.
  expect(at(DEFAULT_LAYOUT, "tasks", "tasks", 200, 590)).toBe("below [] y(x(y(agents usage) chat) tasks)")
  expect(at(DEFAULT_LAYOUT, "tasks", "tasks", 200, 300)).toBeUndefined()
})

test("nested groups take a strip each, widest outermost", () => {
  const deep = y([0.7, 0.3], x([0.6, 0.4], y([0.5, 0.5], "agents", "chat"), "usage"), "tasks")
  // The left edge of Agents is also the left of its stack and of the page.
  expect(at(deep, "usage", "agents", 5, 300)).toBe("left [] x(usage y(agents chat tasks))")
  expect(at(deep, "usage", "agents", 40, 300)).toBe("left [0,0] y(x(usage y(agents chat)) tasks)")
  expect(at(deep, "usage", "agents", 80, 300)).toBe("left [0,0,0] y(x(usage agents) chat tasks)")
  // The bottom of Chat is inside the page: only its row's stack and itself touch it.
  expect(at(deep, "tasks", "chat", 200, 550)).toBe("below [0,0,1] x(y(agents chat tasks) usage)")
  expect(at(deep, "usage", "chat", 200, 590)).toBe("below [0,0,1] y(agents chat usage tasks)")
})

test("a place that does not pass is not offered and takes no room", () => {
  const noFullWidth = (n: Layout) => !shape(n).startsWith("y(")
  expect(at(DEFAULT_LAYOUT, "tasks", "chat", 200, 590, noFullWidth)).toBe("below [1] x(y(agents usage) y(chat tasks))")
  expect(at(DEFAULT_LAYOUT, "tasks", "chat", 200, 590, () => false)).toBeUndefined()
})

const move = (root: Layout, id: PaneId, ...keys: string[]) => keys.reduce<Layout | undefined>((l, k) => l && step(l, id, `Arrow${k}`), root)
const moved = (root: Layout, id: PaneId, ...keys: string[]) => { const n = move(root, id, ...keys); if (n) sound(n); return n && shape(n) }

test("arrow keys move within a stack, into the neighbouring column, and out into a new edge column", () => {
  expect(moved(DEFAULT_LAYOUT, "agents", "Down")).toBe("x(y(usage agents) chat tasks)")
  expect((move(DEFAULT_LAYOUT, "agents", "Down") as Split & { children: Split[] }).children[0]!.sizes).toEqual([0.35, 0.65])
  expect(moved(DEFAULT_LAYOUT, "usage", "Right")).toBe("x(agents y(chat usage) tasks)")
  expect(moved(DEFAULT_LAYOUT, "chat", "Left")).toBe("x(y(chat agents usage) tasks)")
  expect(moved(DEFAULT_LAYOUT, "usage", "Left")).toBe("x(usage agents chat tasks)")
  expect(move(DEFAULT_LAYOUT, "tasks", "Right")).toBeUndefined() // already along that edge
  expect(step(DEFAULT_LAYOUT, "tasks", "x")).toBeUndefined()
})

test("arrow keys reach the spanning places", () => {
  expect(moved(DEFAULT_LAYOUT, "tasks", "Down")).toBe("y(x(y(agents usage) chat) tasks)")
  expect(moved(DEFAULT_LAYOUT, "agents", "Up")).toBe("y(agents x(usage chat tasks))")
  const under = move(DEFAULT_LAYOUT, "tasks", "Down")!
  expect(moved(under, "usage", "Right", "Right", "Right")).toBe("x(y(x(agents chat) tasks) usage)")
})

const shut = x([0.5, 0.5], y([0.5, 0, 0.5], "agents", "usage", "chat"), "tasks")

test("a shut Usage reopens; left without its stack it opens; it cannot be swapped with", () => {
  expect((reopen(shut) as Split & { children: Split[] }).children[0]!.sizes).toEqual([0.325, 0.35, 0.325])
  expect(to(shut, "tasks", "usage", "swap")).toBeUndefined()
  let next = to(shut, "agents", "tasks", "above")!
  next = to(next, "chat", "tasks", "above")!
  expect(next).toMatchObject({ children: ["usage", { axis: "y" }] }) // open, beside the others
  sound(next)
  sound(to(shut, "tasks", "usage", "above")) // a shut target gives the newcomer a share
})

test("a layout fits only when every pane keeps its minimum size", () => {
  const four = to(DEFAULT_LAYOUT, "usage", "chat", "right")!
  expect(fits(four, DEFAULT_LAYOUT, 1000, 800, 8)).toBe(true) // 964px
  expect(fits(four, DEFAULT_LAYOUT, 900, 800, 8)).toBe(false)
  const stack = to(DEFAULT_LAYOUT, "chat", "usage", "below")!
  expect(fits(stack, DEFAULT_LAYOUT, 1000, 400, 8)).toBe(true) // 3 * 128 + 16
  expect(fits(stack, DEFAULT_LAYOUT, 1000, 399, 8)).toBe(false)
  const under = dock(DEFAULT_LAYOUT, "tasks", [], "below")! // a nested stack under a row: 264 + 8 + 128
  expect(fits(under, DEFAULT_LAYOUT, 1000, 400, 8)).toBe(true)
  expect(fits(under, DEFAULT_LAYOUT, 1000, 399, 8)).toBe(false)
  expect(fits(DEFAULT_LAYOUT, four, 700, 800, 8)).toBe(true) // too small already: needing less is allowed
})

test("settling holds panes at their minimum, and a pane's area follows from the sizes", () => {
  const next = settle(to(DEFAULT_LAYOUT, "usage", "agents", "right")!, 1016, 808, 8) as Split
  expect(next.sizes[1]! * 992).toBeCloseTo(220) // half of a narrow column is held at the minimum
  sound(next)
  const rect = (l: Layout, id: PaneId, w: number) => { const r = paneRect(l, id, w, 808, 8); return [r.left, r.top, r.width, r.height].map(Math.round) }
  expect(rect(DEFAULT_LAYOUT, "usage", 1016)).toEqual([0, 528, 220, 280])
  expect(rect(shut, "chat", 1008)).toEqual([0, 408, 500, 400])
  expect(rect(shut, "agents", 1008)).toEqual([0, 0, 500, 368]) // its cell also holds the button
})

test("a saved layout round-trips and undefined removes it", () => {
  const layout = dock(DEFAULT_LAYOUT, "tasks", [], "below")!
  savePanes(layout)
  expect(loadPanes()).toEqual(layout)
  expect(JSON.parse(localStorage.getItem("tagent.panes")!).v).toBe(4)
  savePanes(undefined)
  expect(loadPanes()).toBeUndefined()
})

const saved = (value: object) => { localStorage.setItem("tagent.panes", JSON.stringify(value)); return loadPanes() }
const v2 = (extra: object) => saved({ v: 2, cols: [0.3, 0.4, 0.3], left: [0.5, 0.5], ...extra })
const columns = [
  { width: 0.22, panes: ["agents", "usage"], heights: [0.65, 0.35] },
  { width: 0.43, panes: ["chat"], heights: [1] },
  { width: 0.35, panes: ["tasks"], heights: [1] },
]
const v3 = (c: unknown) => ({ v: 3, columns: c })

test("a v:3 layout becomes the same columns", () => {
  expect(saved(v3(columns))).toEqual(DEFAULT_LAYOUT)
  expect(saved(v3([{ width: 1, panes: ["agents", "usage", "chat", "tasks"], heights: [0.5, 0, 0.25, 0.25] }])))
    .toEqual(y([0.5, 0, 0.25, 0.25], "agents", "usage", "chat", "tasks")) // one column is a stack; Usage stays shut
})

test("a v:2 layout becomes the same columns", () => {
  expect(v2({})).toEqual(x([0.3, 0.4, 0.3], y([0.5, 0.5], "agents", "usage"), "chat", "tasks"))
  expect(shape(v2({ order: ["tasks", "chat", "usage", "agents"] })!)).toBe("x(y(tasks chat) usage agents)")
  expect(shape(v2({ order: ["chat", "chat", "agents", "tasks"] })!)).toBe("x(y(agents usage) chat tasks)")
  expect(v2({ left: [1, 0] })).toMatchObject({ children: [{ sizes: [1, 0] }, "chat", "tasks"] })
  expect(v2({ left: [1, 0], order: ["agents", "chat", "usage", "tasks"] })).toMatchObject({ children: [{ sizes: [0.65, 0.35] }, "usage", "tasks"] })
})

const v4 = (root: unknown) => ({ v: 4, root })
const good = DEFAULT_LAYOUT

test.each([
  "{bad",
  "5",
  "null",
  { v: 1, boxes: [{ id: "usage", x: 0, y: 0, w: 3, h: 3 }] }, // the old grid boxes
  { v: 2, cols: [0.5, 0.5], left: [0.65, 0.35] },
  { v: 2, cols: [0.22, 0.43, 0.35], left: [0.65] },
  { v: 2, cols: [0.9, 0.9, 0.9], left: [0.65, 0.35] }, // does not add up to 1
  { v: 2, cols: [0, 0.5, 0.5], left: [0.65, 0.35] },
  v3(null),
  v3([]),
  v3([null]),
  v3(columns.slice(1)), // panes missing
  v3([...columns, columns[2]]), // a pane twice
  v3([{ ...columns[0], heights: [0, 1] }, columns[1], columns[2]]), // only Usage shuts
  { v: 5, root: good },
  v4(null),
  v4("chat"),
  v4({ axis: "z", sizes: good.sizes, children: good.children }),
  v4({ ...good, sizes: [0.5, 0.5] }),
  v4({ ...good, sizes: [0.9, 0.9, 0.9] }),
  v4({ ...good, children: good.children.slice(1), sizes: [0.5, 0.5] }),
  v4({ ...good, children: [good.children[0], "chat", "chat"] }),
  v4({ ...good, children: [good.children[0], "chat", "nope"] }),
  v4({ ...good, children: [good.children[0], "chat", { axis: "y", sizes: [], children: [] }] }),
  v4(x([0, 0.5, 0.5], "usage", "chat", y([0.5, 0.5], "agents", "tasks"))), // Usage shuts only in a stack
])("corrupt or unknown value %j gives no layout", value => {
  localStorage.setItem("tagent.panes", typeof value === "string" ? value : JSON.stringify(value))
  expect(loadPanes()).toBeUndefined()
})

test("a saved tree is normalized", () => {
  expect(saved(v4(x([0.5, 0.5], x([0.5, 0.5], "agents", "usage"), y([1], x([0.4, 0.6], "chat", "tasks"))))))
    .toEqual(x([0.25, 0.25, 0.2, 0.3], "agents", "usage", "chat", "tasks"))
})

test("the default is sound", () => sound(DEFAULT_LAYOUT))

test("throwing storage is ignored", () => {
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("quota") })
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("denied") })
  expect(() => savePanes(DEFAULT_LAYOUT)).not.toThrow()
  expect(loadPanes()).toBeUndefined()
  vi.restoreAllMocks()
})
