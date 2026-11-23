// Chat view data: the harness transcript as the server sends it, tool calls merged with their results.

export interface Tool { name?: string; label?: string; callId?: string; input?: string; output?: string }
export interface Item { id: string; role: "user" | "assistant" | "tool" | "thinking"; text?: string; tool?: Tool; time?: string }
export interface Feed {
  harness: string
  status: string
  items: Item[]
  cursor: string
  reset: boolean
  more: boolean
  unsupported?: boolean
  unreadable?: boolean
  queued: string[]
  screen?: string
}

export type Row = { kind: "msg"; item: Item } | { kind: "tool"; call: Item; output?: string }

// A tool call has a label; its result carries only the output (matched by callId).
export function toRows(items: Item[]): Row[] {
  const rows: Row[] = []
  const calls = new Map<string, Extract<Row, { kind: "tool" }>>()
  for (const it of items) {
    if (it.role !== "tool" || !it.tool) { rows.push({ kind: "msg", item: it }); continue }
    if (it.tool.label) {
      const row = { kind: "tool" as const, call: it }
      rows.push(row)
      if (it.tool.callId) calls.set(it.tool.callId, row)
    } else {
      const call = it.tool.callId ? calls.get(it.tool.callId) : undefined
      if (call) call.output = it.tool.output ?? ""
    }
  }
  return rows
}

// The tool the agent is running now: the last call still waiting for its result.
export function runningTool(rows: Row[]): string | undefined {
  const last = rows[rows.length - 1]
  if (last?.kind === "tool" && last.output === undefined) return last.call.tool?.label
  return undefined
}

const key = (pane: string) => `tagent.display.${pane}`
export type DisplayMode = "terminal" | "chat"

// Remembered per agent and device; storage can be blocked, so every access is guarded.
// Device-wide default from Settings: "auto" picks Chat on phones, Terminal elsewhere.
export type DisplayDefault = "auto" | DisplayMode
const DEFAULT_KEY = "tagent.agentDisplay"
export function getDisplayDefault(): DisplayDefault {
  let v: string | null = null
  try { v = localStorage.getItem(DEFAULT_KEY) } catch { /* blocked storage */ }
  return v === "terminal" || v === "chat" ? v : "auto"
}
export function setDisplayDefault(v: DisplayDefault) {
  try { localStorage.setItem(DEFAULT_KEY, v) } catch { /* not remembered */ }
}

export function getMode(pane: string, chatSupported: boolean): DisplayMode {
  if (!chatSupported) return "terminal"
  let saved: string | null = null
  try { saved = localStorage.getItem(key(pane)) } catch { /* private window or blocked storage */ }
  if (saved === "terminal" || saved === "chat") return saved
  const fallback = getDisplayDefault()
  if (fallback !== "auto") return fallback
  return typeof matchMedia === "function" && matchMedia("(max-width: 767px)").matches ? "chat" : "terminal"
}

export function setMode(pane: string, mode: DisplayMode) {
  try { localStorage.setItem(key(pane), mode) } catch { /* not remembered */ }
}
