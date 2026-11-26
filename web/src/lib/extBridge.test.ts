import { beforeEach, expect, test, vi } from "vitest"
import { api } from "@/api"
import { extReply } from "./extBridge"

vi.mock("@/api", () => ({ api: vi.fn() }))

const frame = { postMessage: vi.fn() } as unknown as Window
const msg = (data: unknown, source: unknown = frame) => ({ data, source }) as unknown as MessageEvent

beforeEach(() => {
  vi.mocked(api).mockReset().mockResolvedValue({ host: "box" })
})

test("ignores anything that is not a bridge call from this pane", () => {
  expect(extReply(msg({ tagent: "api", id: 1, path: "/api/state" }, {}), frame)).toBeUndefined()
  expect(extReply(msg({ tagent: "api", id: 1, path: "/api/state" }), null)).toBeUndefined()
  expect(extReply(msg({ tagent: "other", id: 1, path: "/api/state" }), frame)).toBeUndefined()
  expect(extReply(msg({ tagent: "api", id: 1 }), frame)).toBeUndefined()
  expect(extReply(msg(null), frame)).toBeUndefined()
  expect(api).not.toHaveBeenCalled()
})

test.each(["/ext/x", "/api/../x", "//evil.example/api/x", "https://evil.example/api/x", "/apix"])("rejects %s", async path => {
  expect(await extReply(msg({ tagent: "api", id: 7, path }), frame)).toMatchObject({ tagent: "reply", id: 7, error: expect.any(String) })
  expect(api).not.toHaveBeenCalled()
})

test("forwards GET and POST to api, with the query and the JSON body", async () => {
  expect(await extReply(msg({ tagent: "api", id: 1, path: "/api/state?x=1" }), frame)).toEqual({ tagent: "reply", id: 1, data: { host: "box" } })
  expect(api).toHaveBeenLastCalledWith("/api/state?x=1", { method: "GET" })
  await extReply(msg({ tagent: "api", id: 2, path: "/api/ext/hello/data", method: "PUT", body: { n: 1 } }), frame)
  expect(api).toHaveBeenLastCalledWith("/api/ext/hello/data", { method: "PUT", headers: { "Content-Type": "application/json" }, body: '{"n":1}' })
})

test("reports an API error back to the pane", async () => {
  vi.mocked(api).mockRejectedValueOnce(new Error("unauthorized: open the link"))
  expect(await extReply(msg({ tagent: "api", id: 3, path: "/api/state" }), frame)).toEqual({ tagent: "reply", id: 3, error: "unauthorized: open the link" })
})
