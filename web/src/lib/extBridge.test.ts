import { beforeEach, expect, test, vi } from "vitest"
import { api } from "@/api"
import { canonical, extReply } from "./extBridge"

vi.mock("@/api", () => ({ api: vi.fn() }))

const frame = { postMessage: vi.fn() } as unknown as Window
const msg = (data: unknown, source: unknown = frame) => ({ data, source }) as unknown as MessageEvent
const allow = vi.fn()
// call sends one bridge message from pane "hello"; canAct says whether the user allowed actions (or the pane is the user's own).
const call = (path: string, method?: string, body?: unknown, id = 1) => extReply(msg({ tagent: "api", id, path, method, body }), frame, "hello", allow)

beforeEach(() => {
  vi.mocked(api).mockReset().mockResolvedValue({ host: "box" })
  allow.mockReset().mockResolvedValue(true)
})

test("ignores anything that is not a bridge call from this pane", () => {
  expect(extReply(msg({ tagent: "api", id: 1, path: "/api/state" }, {}), frame, "hello", allow)).toBeUndefined()
  expect(extReply(msg({ tagent: "api", id: 1, path: "/api/state" }), null, "hello", allow)).toBeUndefined()
  expect(extReply(msg({ tagent: "other", id: 1, path: "/api/state" }), frame, "hello", allow)).toBeUndefined()
  expect(extReply(msg({ tagent: "api", id: 1 }), frame, "hello", allow)).toBeUndefined()
  expect(extReply(msg(null), frame, "hello", allow)).toBeUndefined()
  expect(api).not.toHaveBeenCalled()
})

test.each(["/ext/x", "/api/../x", "//evil.example/api/x", "https://evil.example/api/x", "/apix"])("rejects %s", async path => {
  expect(await call(path, undefined, undefined, 7)).toMatchObject({ tagent: "reply", id: 7, error: expect.any(String) })
  expect(api).not.toHaveBeenCalled()
})

test("forwards GET and POST to api, with the query and the JSON body", async () => {
  expect(await call("/api/state?x=1")).toEqual({ tagent: "reply", id: 1, data: { host: "box" } })
  expect(api).toHaveBeenLastCalledWith("/api/state?x=1", { method: "GET" })
  await call("/api/ext/hello/data", "PUT", { n: 1 }, 2)
  expect(api).toHaveBeenLastCalledWith("/api/ext/hello/data", { method: "PUT", headers: { "Content-Type": "application/json" }, body: '{"n":1}' })
  await call("/api/pane/send", "POST", { pane: "%1", data: "x" }, 3)
  expect(api).toHaveBeenLastCalledWith("/api/pane/send", { method: "POST", headers: { "Content-Type": "application/json" }, body: '{"pane":"%1","data":"x"}' })
})

test("reports an API error back to the pane", async () => {
  vi.mocked(api).mockRejectedValueOnce(new Error("unauthorized: open the link"))
  expect(await call("/api/state")).toEqual({ tagent: "reply", id: 1, error: "unauthorized: open the link" })
})

// (A dot segment such as "/api/ext/%2e%2e/other/data" is resolved by the URL parser to /api/other/data before the checks: not under ext.)
// Pane management, grants and other panes' data are out of reach for every pane, even one with full access.
test.each([
  "/api/ext", "/api/ext/", "/api/ext?x=1", "/api/ext/other/data", "/api/ext/hello", "/api/ext/hello/grant", "/api/ext/other/grant",
  "/api/ext/hello/data/x", "/api/ext/hello/data/../../other/data", "/api/ext/hello/../other/data", "/api/x/../ext/other/data",
  "/api/ext/hello/%2E%2E/other/data", "/api/ext/hello/%2e%2e/%2e%2e/ext", "/api/ext/hello/..%2fother/data", "/api/ext/hello%2f..%2fother/data",
  "/api/%65xt/other/data", "/api/%45xt", "/api/ext%2Fother%2Fdata", "/api/ext%2Fhello%2Fgrant", "/api/%2565xt/other/data", "/api/ext/hello/%252e%252e/other/data",
  "/api/ext/hello/data%2f..%2f..%2fother%2fdata", "/api/EXT/other/data", "/api/Ext", "/api/ext%00", "/api/ext;x=1", "/api/ext/other/data#hello",
  "/api\\ext\\other\\data", "/api/%5cext/other/data", "/api//ext/other/data", "/api/./ext/other/data", "/api/ext/./other/data", "/api/ext/hello/data/", "/api/ext/hello/data%2f",
  "/api/ext/%68ello/data/../../other/data", "/api/ext/hello/data/..%2f..%2fother/data", "/api/ext/hello/dat%61/x", "/api/ext/hell%6f/grant",
])("a pane cannot reach %s", async path => {
  for (const method of [undefined, "GET", "PUT", "POST", "DELETE", "get", "put"]) {
    const r = await call(path, method)
    expect(r, `${method} ${path}`).toMatchObject({ error: expect.any(String) })
    expect(r).not.toHaveProperty("data")
  }
  expect(api).not.toHaveBeenCalled()
})

test("a pane reaches its own data, exactly", async () => {
  expect(await call("/api/ext/hello/data")).toMatchObject({ data: { host: "box" } })
  expect(await call("/api/ext/hello/data?a=1", "PUT", { x: 1 })).toMatchObject({ data: { host: "box" } })
  expect(api).toHaveBeenCalledTimes(2)
  // A name that is only a prefix, or another case, is another pane.
  expect(await call("/api/ext/hello2/data")).toMatchObject({ error: expect.any(String) })
  expect(await call("/api/ext/Hello/data")).toMatchObject({ error: expect.any(String) })
  expect(api).toHaveBeenCalledTimes(2)
})

test("a read-only pane may GET, and use its own data, nothing else", async () => {
  allow.mockResolvedValue(false)
  expect(await call("/api/state")).toMatchObject({ data: { host: "box" } })
  expect(await call("/api/state", "get")).toMatchObject({ data: { host: "box" } })
  expect(await call("/api/ext/hello/data", "PUT", { n: 1 })).toMatchObject({ data: { host: "box" } })
  expect(api).toHaveBeenCalledTimes(3)
  for (const [path, method] of [["/api/pane/send", "POST"], ["/api/pane/kill", "post"], ["/api/agents", "POST"], ["/api/workflows", "DELETE"], ["/api/routing", "PUT"], ["/api/state", "PUT"], ["/api/state", "PATCH"], ["/api/state", "patch"], ["/api/state", "HEAD"], ["/api/state", "OPTIONS"], ["/api/state", "DELETE"], ["/api/state", "X-HTTP-Method-Override"]]) {
    expect(await call(path!, method), `${method} ${path}`).toMatchObject({ error: expect.stringContaining("read-only") })
  }
  expect(api).toHaveBeenCalledTimes(3)
})

test("an allowed pane may write; the check runs at each write, and a failed check denies", async () => {
  expect(await call("/api/pane/send", "POST", { pane: "%1" })).toMatchObject({ data: { host: "box" } })
  expect(allow).toHaveBeenCalledTimes(1)
  allow.mockResolvedValueOnce(false)
  expect(await call("/api/pane/send", "POST", {})).toMatchObject({ error: expect.stringContaining("read-only") })
  allow.mockRejectedValueOnce(new Error("down"))
  expect(await call("/api/pane/send", "POST", {})).toMatchObject({ error: expect.any(String) })
  expect(api).toHaveBeenCalledTimes(1)
})

test("canonical decodes, repeatedly, and resolves dots and slashes", () => {
  expect(canonical("/api/a/../b")).toBe("/api/b")
  expect(canonical("/api/%252e%252e/x")).toBe("/x")
  expect(canonical("/api//a///b/")).toBe("/api/a/b")
  expect(canonical("/api\\a")).toBe("/api/a")
  expect(canonical("/api/agents/%255/transcript")).toBe("/api/agents/%5/transcript") // a tmux pane id stays
  expect(canonical("/api/%zz")).toBeUndefined()
})
