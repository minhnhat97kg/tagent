import { post } from "@/api"
import { toast } from "sonner"

// A fixed id makes sonner replace one toast instead of stacking one per rejected key.
export function reportInputError(e: unknown) {
  toast.error((e as Error).message, { id: "terminal-input" })
}

type Waiter = { resolve: () => void; reject: (e: Error) => void }

// paneChannel is one WebSocket's input FIFO. Each key is written as soon as it is sent; the next ack
// resolves the oldest waiter. Closing the socket rejects every waiter, and bumps the epoch so input
// still held for it never reaches a later socket.
export function paneChannel(write: (data: string) => boolean) {
  const waiting: Waiter[] = []
  let epoch = 0
  return {
    epoch: () => epoch,
    send(data: string, sentIn: number): Promise<void> {
      return new Promise<void>((resolve, reject) => {
        if (sentIn !== epoch || !write(data)) reject(new Error("Terminal disconnected"))
        else waiting.push({ resolve, reject })
      })
    },
    ack(ok: boolean) {
      const w = waiting.shift()
      if (!w) return
      if (ok) w.resolve()
      else w.reject(new Error("Terminal input failed; delivery uncertain"))
    },
    close() {
      epoch++
      const err = new Error("Terminal disconnected; delivery uncertain, not retried")
      for (const w of waiting.splice(0)) w.reject(err)
    },
  }
}

export type PaneChannel = ReturnType<typeof paneChannel>

type Lane = { issue: Promise<unknown>; inflight: Set<Promise<void>> }
const lanes = new Map<string, Lane>()
const sockets = new Map<string, PaneChannel>()

function lane(pane: string) {
  let l = lanes.get(pane)
  if (!l) lanes.set(pane, l = { issue: Promise.resolve(), inflight: new Set() })
  return l
}

export function registerPaneSocket(pane: string, channel: PaneChannel) {
  sockets.set(pane, channel)
  return () => { if (sockets.get(pane) === channel) sockets.delete(pane) }
}

// Ordering per pane: input keeps call order and is written without waiting for earlier acks. An
// answer, send, or HTTP-fallback input starts only after every earlier socket input is acked, and
// later input waits for it. A failed step still lets the next one run. Input is never retried.
export function paneInput(pane: string, action: "input" | "answer" | "send", data: string, extra = {}): Promise<unknown> {
  const l = lane(pane)
  const socket = sockets.get(pane)
  if (action === "input" && socket) {
    const epoch = socket.epoch()
    let sent: Promise<void> = Promise.resolve()
    const issued = l.issue.then(() => { sent = socket.send(data, epoch) })
    l.issue = issued
    const done = issued.then(() => sent)
    l.inflight.add(done)
    const forget = () => { l.inflight.delete(done) }
    done.then(forget, forget)
    return done
  }
  const earlier = [...l.inflight]
  const op = l.issue.then(async () => {
    await Promise.allSettled(earlier)
    return post(`/api/pane/${action}`, { pane, data, ...extra })
  })
  l.issue = op.then(() => {}, () => {})
  return op
}
