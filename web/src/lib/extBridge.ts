import { api } from "@/api"

// The bridge for custom panes: a pane (a sandboxed iframe, see internal/web/ext_sdk.js) asks for /api/* paths,
// and the app runs them with its own cookie. The reply goes back to the pane's window.
export type ExtReply = { tagent: "reply"; id: unknown; data?: unknown; error?: string }

// extReply answers one message from the pane `frame`. Undefined when the message is not a bridge call from it.
export function extReply(event: MessageEvent, frame: Window | null): Promise<ExtReply> | undefined {
  const d = event.data as { tagent?: unknown; id?: unknown; path?: unknown; method?: unknown; body?: unknown } | null
  if (!frame || event.source !== frame || !d || d.tagent !== "api" || typeof d.path !== "string") return undefined
  const reply = (r: { data?: unknown; error?: string }): ExtReply => ({ tagent: "reply", id: d.id, ...r })
  let url: URL
  try {
    url = new URL(d.path, location.origin)
  } catch {
    return Promise.resolve(reply({ error: `bad path: ${d.path}` }))
  }
  // Same origin and /api/ only: this rejects "/api/../x", "//host/api/x" and absolute URLs to other hosts.
  if (url.origin !== location.origin || !url.pathname.startsWith("/api/")) {
    return Promise.resolve(reply({ error: `only /api/ paths: ${d.path}` }))
  }
  const init: RequestInit = { method: typeof d.method === "string" ? d.method : "GET" }
  if (d.body !== undefined) {
    init.headers = { "Content-Type": "application/json" }
    init.body = JSON.stringify(d.body)
  }
  return api(url.pathname + url.search, init).then(
    data => reply({ data }),
    (e: Error) => reply({ error: e.message }),
  )
}
