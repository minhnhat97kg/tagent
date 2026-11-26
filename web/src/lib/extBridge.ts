import { api } from "@/api"

// The bridge for custom panes: a pane (a sandboxed iframe, see internal/web/ext_sdk.js) asks for /api/* paths,
// and the app runs them with its own cookie. The reply goes back to the pane's window.
//
// The rules, the same for every pane: only /api/ paths of this origin; nothing under /api/ext (pane management,
// grants, other panes' data) except the pane's own /api/ext/<name>/data. A pane an agent made may only GET, and
// use its own data, unless the user allowed actions for its current HTML (canAct).
export type ExtReply = { tagent: "reply"; id: unknown; data?: unknown; error?: string }

// canonical is what a server could take path for, after decoding (repeatedly: %252e), backslashes, repeated
// slashes and dot segments; undefined when it will not decode or is still changing after a few rounds.
export function canonical(path: string): string | undefined {
  let cur = path
  for (let i = 0; i < 5; i++) {
    let next: string
    // A later round that no longer decodes is a literal "%" (a tmux pane id "%5" arrives as %255): stop there.
    try { next = decodeURIComponent(cur) } catch { return i === 0 ? undefined : cur }
    const out: string[] = []
    for (const seg of next.replace(/\\/g, "/").split("/")) {
      if (seg === "..") { if (out.length > 1) out.pop() } else if (seg !== "." && (seg !== "" || out.length === 0)) out.push(seg)
    }
    next = out.join("/")
    if (next === cur) return cur
    cur = next
  }
  return undefined
}

// extReply answers one message from the pane `name` in `frame`. Undefined when the message is not a bridge call from it.
export function extReply(event: MessageEvent, frame: Window | null, name: string, canAct: () => Promise<boolean>): Promise<ExtReply> | undefined {
  const d = event.data as { tagent?: unknown; id?: unknown; path?: unknown; method?: unknown; body?: unknown } | null
  if (!frame || event.source !== frame || !d || d.tagent !== "api" || typeof d.path !== "string") return undefined
  const reply = (r: { data?: unknown; error?: string }): ExtReply => ({ tagent: "reply", id: d.id, ...r })
  const deny = (error: string) => Promise.resolve(reply({ error }))
  let url: URL
  try {
    url = new URL(d.path, location.origin)
  } catch {
    return deny(`bad path: ${d.path}`)
  }
  // Same origin and /api/ only: this rejects "/api/../x", "//host/api/x" and absolute URLs to other hosts.
  if (url.origin !== location.origin || !url.pathname.startsWith("/api/")) return deny(`only /api/ paths: ${d.path}`)
  const canon = canonical(url.pathname)
  if (canon === undefined) return deny(`bad path: ${d.path}`)
  const own = `/api/ext/${name}/data`
  const isOwn = url.pathname === own && canon === own
  if (!isOwn && [url.pathname, canon].some(p => p.toLowerCase().startsWith("/api/ext"))) return deny(`panes cannot reach ${d.path}`)
  const method = typeof d.method === "string" ? d.method.toUpperCase() : "GET"
  const init: RequestInit = { method }
  if (d.body !== undefined) {
    init.headers = { "Content-Type": "application/json" }
    init.body = JSON.stringify(d.body)
  }
  const run = () => api(url.pathname + url.search, init).then(
    data => reply({ data }),
    (e: Error) => reply({ error: e.message }),
  )
  if (isOwn || method === "GET") return run()
  return canAct().then(ok => (ok ? run() : reply({ error: `read-only pane (made by an agent): ${method} not allowed until you allow actions` })), () => reply({ error: "cannot check pane access" }))
}
