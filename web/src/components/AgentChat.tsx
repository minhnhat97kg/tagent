import { useLayoutEffect, useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react"
import { Brain, ExternalLink, FileText, Loader2, Pencil, Search, Send, Terminal, Wrench } from "lucide-react"
import { api } from "@/api"
import { Md } from "@/components/BoardDrawer"
import { Button } from "@/components/ui/button"
import { paneInput } from "@/lib/paneInput"
import { runningTool, toRows, type Feed, type Item, type Row } from "@/lib/transcript"

const MAX_ITEMS = 1000
const POLL_MS = 1500

// AgentChat renders the agent's harness transcript. The agent keeps running in tmux; this only reads it and sends text.
export function AgentChat({ pane, onOpenTerminal, onUnavailable }: { pane: string; onOpenTerminal: () => void; onUnavailable: () => void }) {
  const [items, setItems] = useState<Item[]>([])
  const [feed, setFeed] = useState<Feed>()
  const [draft, setDraft] = useState("")
  const [sending, setSending] = useState(false)
  const [error, setError] = useState("")
  const cursor = useRef("")
  const unavailable = useRef(onUnavailable)
  useEffect(() => { unavailable.current = onUnavailable })
  const scroller = useRef<HTMLDivElement>(null)
  const atBottom = useRef(true)
  const touch = typeof matchMedia === "function" && matchMedia("(pointer: coarse)").matches

  // Poll while visible; a page that is not the last one is fetched again at once.
  useEffect(() => {
    let stopped = false
    let timer = 0
    const tick = async (): Promise<void> => {
      let more = false
      if (document.visibilityState === "visible") {
        try {
          const after = cursor.current ? `?after=${encodeURIComponent(cursor.current)}` : ""
          const f = await api<Feed>(`/api/agents/${encodeURIComponent(pane)}/transcript${after}`)
          if (stopped) return
          setFeed(f)
          setError("")
          // Unreadable is not final: a new agent has no transcript until its first message.
          if (f.unsupported) unavailable.current()
          if (f.reset || f.cursor) cursor.current = f.cursor
          if (f.reset) setItems(f.items)
          else if (f.items.length) setItems(prev => [...prev, ...f.items].slice(-MAX_ITEMS))
          more = f.more
        } catch (e) {
          if (!stopped) setError((e as Error).message)
        }
      }
      if (!stopped) timer = window.setTimeout(() => void tick(), more ? 0 : POLL_MS)
    }
    void tick()
    return () => { stopped = true; clearTimeout(timer) }
  }, [pane])

  // Follow new messages only while the reader is at the bottom.
  useLayoutEffect(() => {
    const el = scroller.current
    if (el && atBottom.current) el.scrollTop = el.scrollHeight
  }, [items, feed?.status])

  const rows = toRows(items)
  const tool = runningTool(rows)

  async function send() {
    const text = draft.trim()
    if (!text || sending) return
    setSending(true)
    setError("")
    try {
      const r = (await paneInput(pane, "send", text)) as { queued: boolean }
      setDraft("")
      atBottom.current = true
      if (r.queued) setFeed(f => f && { ...f, queued: [...f.queued, text] })
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSending(false)
    }
  }

  // Enter sends on desktop; Shift+Enter and IME composition keep their normal meaning.
  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (!touch && e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      void send()
    }
  }

  const waiting = feed?.status === "waiting"
  return <div className="flex min-h-0 flex-1 flex-col">
    <div ref={scroller} role="log" aria-label="Conversation" className="min-h-0 flex-1 space-y-2 overflow-y-auto p-3"
      onScroll={e => { const el = e.currentTarget; atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48 }}>
      {feed?.unsupported && <p className="text-sm text-muted-foreground">Chat is not available for this agent. Use Terminal.</p>}
      {feed?.unreadable && <p className="text-sm text-muted-foreground">The transcript is not readable yet. Use Terminal.</p>}
      {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
      {rows.map(r => r.kind === "msg" ? <Message key={r.item.id} item={r.item} /> : <ToolCard key={r.call.id} row={r} />)}
      {waiting && <div className="rounded-lg border border-l-4 border-l-amber-500 bg-card p-2 text-xs">
        <p className="mb-1 font-medium">Approval needed. Answer it in Terminal.</p>
        <pre className="max-h-48 overflow-auto whitespace-pre-wrap">{feed?.screen}</pre>
        <Button size="sm" variant="outline" className="mt-2 max-md:min-h-11" onClick={onOpenTerminal}><ExternalLink />Open in Terminal</Button>
      </div>}
    </div>
    <p role="status" className="flex items-center gap-1.5 px-3 pt-1 text-xs text-muted-foreground">
      {feed?.status === "running" && <Loader2 className="size-3 animate-spin" />}
      {statusLabel(feed?.status, tool)}
    </p>
    {!!feed?.queued.length && <div className="px-3 pt-1 text-xs text-muted-foreground">
      <p>Queued until the agent is idle ({feed.queued.length}):</p>
      <ul>{feed.queued.map((q, i) => <li key={i} className="truncate">{q}</li>)}</ul>
    </div>}
    <form className="flex items-end gap-2 border-t p-2" onSubmit={e => { e.preventDefault(); void send() }}>
      <textarea aria-label="Message agent" rows={1} value={draft} placeholder="Message this agent"
        onChange={e => setDraft(e.target.value)} onKeyDown={onKeyDown}
        className="max-h-40 min-h-11 flex-1 resize-none rounded-md border bg-background px-3 py-2 text-sm" />
      <Button type="submit" size="icon" aria-label="Send" className="max-md:size-11" disabled={!draft.trim() || sending}><Send /></Button>
    </form>
  </div>
}

function statusLabel(status: string | undefined, tool: string | undefined) {
  switch (status) {
    case "running": return tool ? `Working · ${tool}` : "Working"
    case "background": return "Working in background"
    case "waiting": return "Waiting for approval"
    case "idle": case "done": return "Idle"
    case "error": return "Error"
    default: return status ?? ""
  }
}

function Message({ item }: { item: Item }) {
  if (item.role === "thinking") return <details className="rounded-md border px-2 py-1 text-xs text-muted-foreground">
    <summary className="cursor-pointer"><Brain className="inline size-3" /> Reasoning</summary>
    <p className="mt-1 whitespace-pre-wrap">{item.text}</p>
  </details>
  const mine = item.role === "user"
  return <div className={`max-w-[85%] rounded-lg px-3 py-2 text-sm ${mine ? "ml-auto bg-primary text-primary-foreground" : "border bg-card"}`}>
    <Md text={item.text ?? ""} />
  </div>
}

function toolIcon(name = ""): ReactNode {
  switch (name.toLowerCase()) {
    case "bash": return <Terminal className="size-3.5 shrink-0" />
    case "edit": case "multiedit": case "write": case "notebookedit": return <Pencil className="size-3.5 shrink-0" />
    case "read": return <FileText className="size-3.5 shrink-0" />
    case "grep": case "glob": return <Search className="size-3.5 shrink-0" />
    default: return <Wrench className="size-3.5 shrink-0" />
  }
}

// ToolCard: one line; tap to see the input and the output.
function ToolCard({ row }: { row: Extract<Row, { kind: "tool" }> }) {
  const tool = row.call.tool ?? {}
  return <details className="rounded-md border bg-card text-xs">
    <summary className="flex cursor-pointer list-none items-center gap-2 px-2 py-1.5">
      {toolIcon(tool.name)}
      <span className="min-w-0 truncate font-mono">{tool.label}</span>
      {row.output === undefined && <Loader2 className="ml-auto size-3 shrink-0 animate-spin" aria-label="running" />}
    </summary>
    <div className="space-y-1 border-t p-2">
      {tool.input && tool.input !== "{}" && <pre className="max-h-40 overflow-auto whitespace-pre-wrap font-mono">{tool.input}</pre>}
      {row.output !== undefined && <pre className="max-h-64 overflow-auto whitespace-pre-wrap font-mono">{row.output || "(no output)"}</pre>}
    </div>
  </details>
}
