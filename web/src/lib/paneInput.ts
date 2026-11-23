import { post } from "@/api"

const queues = new Map<string, Promise<unknown>>()
const sockets = new Map<string, (data: string) => Promise<void>>()
export function registerPaneSocket(pane: string, send: (data: string) => Promise<void>) {
  sockets.set(pane, send)
  return () => { if (sockets.get(pane) === send) sockets.delete(pane) }
}
// Share ordering across raw keys, messages and approvals. Never retry input.
export function paneInput(pane: string, action: "input" | "answer", data: string, extra = {}) {
  const next = (queues.get(pane) ?? Promise.resolve()).catch(() => {}).then(() =>
    action === "input" && sockets.has(pane)
      ? sockets.get(pane)!(data)
      : post(`/api/pane/${action}`, { pane, data, ...extra }),
  )
  queues.set(pane, next)
  void next.finally(() => { if (queues.get(pane) === next) queues.delete(pane) }).catch(() => {})
  return next
}
