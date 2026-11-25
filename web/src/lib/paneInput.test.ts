import { afterEach, expect, test, vi } from "vitest"
import { post } from "@/api"
import { toast } from "sonner"
import { paneChannel, paneInput, registerPaneSocket, reportInputError } from "./paneInput"

vi.mock("@/api", () => ({ post: vi.fn(async () => ({ queued: false })) }))
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }))

const flush = () => new Promise(resolve => setTimeout(resolve, 0))
let pane = 0
const nextPane = () => `%${++pane}`

afterEach(() => { vi.clearAllMocks() })

test("keystrokes reach the socket before the first ack, and send waits for the last ack", async () => {
  const id = nextPane()
  const writes: string[] = []
  const channel = paneChannel(data => { writes.push(data); return true })
  registerPaneSocket(id, channel)
  const keys = ["a", "b", "c", "d", "e"].map(k => paneInput(id, "input", k))
  const sent = paneInput(id, "send", "hello")
  await flush()
  expect(writes).toEqual(["a", "b", "c", "d", "e"])
  expect(post).not.toHaveBeenCalled()
  for (let i = 0; i < 4; i++) channel.ack(true)
  await flush()
  expect(post).not.toHaveBeenCalled() // one ack still outstanding
  channel.ack(true)
  await expect(Promise.all(keys)).resolves.toHaveLength(5)
  await sent
  expect(post).toHaveBeenCalledWith(`/api/pane/send`, { pane: id, data: "hello" })
})

test("typing right after a send settles still writes in call order", async () => {
  const id = nextPane()
  const writes: string[] = []
  const channel = paneChannel(data => { writes.push(data); return true })
  registerPaneSocket(id, channel)
  await paneInput(id, "send", "hello")
  const late = paneInput(id, "input", "z")
  await flush()
  expect(writes).toEqual(["z"])
  channel.ack(true)
  await expect(late).resolves.toBeUndefined()
})

test("a disconnect rejects every pending key, and each reports through one toast id", async () => {
  const id = nextPane()
  const channel = paneChannel(() => true)
  registerPaneSocket(id, channel)
  const keys = Array.from({ length: 20 }, (_, i) => paneInput(id, "input", String(i)))
  await flush()
  channel.close()
  const results = await Promise.allSettled(keys)
  expect(results.every(r => r.status === "rejected")).toBe(true)
  for (const r of results) if (r.status === "rejected") reportInputError(r.reason)
  const calls = vi.mocked(toast.error).mock.calls
  expect(calls).toHaveLength(20)
  expect(new Set(calls.map(c => (c[1] as { id: string }).id))).toEqual(new Set(["terminal-input"]))
})

test("input held behind a send never reaches the socket after a reconnect", async () => {
  const id = nextPane()
  const writes: string[] = []
  const channel = paneChannel(data => { writes.push(data); return true })
  registerPaneSocket(id, channel)
  const first = paneInput(id, "input", "1")
  await flush()
  const sent = paneInput(id, "send", "held")
  const late = paneInput(id, "input", "2")
  await flush()
  channel.close() // the socket drops; first is rejected, the send still runs
  await expect(first).rejects.toThrow("delivery uncertain")
  await sent
  channel.ack(true) // a stale ack for the closed socket must not resolve anything
  await expect(late).rejects.toThrow("Terminal disconnected")
  expect(writes).toEqual(["1"])
})

test("a failed ack rejects that key and later input still runs", async () => {
  const id = nextPane()
  const writes: string[] = []
  const channel = paneChannel(data => { writes.push(data); return true })
  registerPaneSocket(id, channel)
  const bad = paneInput(id, "input", "x")
  await flush()
  channel.ack(false)
  await expect(bad).rejects.toThrow("delivery uncertain")
  const sent = paneInput(id, "send", "after")
  await flush()
  expect(post).toHaveBeenCalledWith(`/api/pane/send`, { pane: id, data: "after" })
  await sent
  expect(writes).toEqual(["x"])
})

test("without a socket, input falls back to HTTP one request at a time", async () => {
  const id = nextPane()
  const order: string[] = []
  vi.mocked(post).mockImplementation(async (_path, body) => {
    const data = (body as { data: string }).data
    order.push(`start ${data}`)
    await flush()
    order.push(`end ${data}`)
    return {}
  })
  await Promise.all([paneInput(id, "input", "a"), paneInput(id, "input", "b")])
  expect(order).toEqual(["start a", "end a", "start b", "end b"])
})
