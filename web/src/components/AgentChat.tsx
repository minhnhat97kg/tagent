import { useLayoutEffect, useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react"
import { ArrowRight, ArrowUp, Brain, ImagePlus, X, Check, ChevronDown, Cpu, ExternalLink, FileText, MessageCircleQuestion, Loader2, Pencil, Search, Terminal, Wrench } from "lucide-react"
import { api, post } from "@/api"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Bubble, Composer, Fold, Markup, chatColumn, composerInput, sendButton } from "@/components/ChatParts"
import { paneInput } from "@/lib/paneInput"
import { currentModel, runningTool, toRows, type Feed, type Item, type Row } from "@/lib/transcript"
import { commandMatches, harnessModels } from "@/lib/harnessCommands"
import { splitImages, uploadImage, type Upload } from "@/lib/uploads"

const MAX_ITEMS = 1000
const POLL_MS = 1500

// AgentChat renders the agent's harness transcript. The agent keeps running in tmux; this only reads it and sends text.
export function AgentChat({ pane, onOpenTerminal, onUnavailable }: { pane: string; onOpenTerminal: () => void; onUnavailable: () => void }) {
  const [items, setItems] = useState<Item[]>([])
  const [feed, setFeed] = useState<Feed>()
  const [draft, setDraft] = useState("")
  const [sending, setSending] = useState(false)
  const [error, setError] = useState("")
  const [pick, setPick] = useState(0)
  const [picked, setPicked] = useState<number>()
  const [images, setImages] = useState<Upload[]>([])
  const [uploading, setUploading] = useState(0)
  const fileInput = useRef<HTMLInputElement>(null)
  const cursor = useRef("")
  const kick = useRef(() => {}) // poll again soon, e.g. right after a key was sent
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
    kick.current = () => { clearTimeout(timer); timer = window.setTimeout(() => void tick(), 400) }
    return () => { stopped = true; clearTimeout(timer); kick.current = () => {} }
  }, [pane])

  // Follow new messages only while the reader is at the bottom.
  useLayoutEffect(() => {
    const el = scroller.current
    if (el && atBottom.current) el.scrollTop = el.scrollHeight
  }, [items, feed?.status])

  // The question is gone from the screen once it is answered: forget the choice so the next one is clickable.
  const askKey = feed?.ask ? feed.ask.question + "|" + feed.ask.options.map(o => o.label).join(",") : ""
  useEffect(() => { setPicked(undefined) }, [askKey])

  const rows = toRows(items)
  const tool = runningTool(rows)

  async function send() {
    // Attached images go in as their file paths, one per line, which is how an agent opens them.
    const text = [draft.trim(), ...images.map(i => i.path)].filter(Boolean).join("\n")
    if (!text || sending || uploading) return
    setSending(true)
    setError("")
    try {
      const r = (await paneInput(pane, "send", text)) as { queued: boolean }
      setDraft("")
      setImages([])
      atBottom.current = true
      if (r.queued) setFeed(f => f && { ...f, queued: [...f.queued, text] })
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSending(false)
    }
  }

  async function attach(files: File[]) {
    const pics = files.filter(f => f.type.startsWith("image/"))
    if (!pics.length) return
    setError("")
    setUploading(n => n + pics.length)
    for (const f of pics) {
      try { const u = await uploadImage(f); setImages(prev => [...prev, u]) } catch (e) { setError((e as Error).message) } finally { setUploading(n => n - 1) }
    }
  }

  // Enter sends on desktop; Shift+Enter and IME composition keep their normal meaning.
  const commands = commandMatches(feed?.harness, draft)
  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (commands.length && !e.nativeEvent.isComposing) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault()
        setPick(p => (p + (e.key === "ArrowDown" ? 1 : commands.length - 1)) % commands.length)
        return
      }
      if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey)) { e.preventDefault(); setDraft(commands[pick % commands.length]![0]); return }
    }
    if (!touch && e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      void send()
    }
  }

  // Answering a question is the user's choice from the chat; approvals still go to Terminal.
  // Keys go straight to the server: they must not depend on the (hidden) Terminal's socket.
  const key = (data: string) => post("/api/pane/input", { pane, data })
  const sleep = (ms: number) => new Promise(r => setTimeout(r, ms))
  async function run(f: () => Promise<unknown>) {
    setError("")
    try { await f() } catch (e) { setError((e as Error).message); setPicked(undefined) }
    kick.current()
  }
  const actions: AskActions = {
    onOpenTerminal,
    // One number key: selects the option, or ticks/unticks it in a multi-select.
    onPick: n => { if (!feed?.ask?.multi) setPicked(n); void run(() => key(String(n))) },
    // Right arrow leaves a multi-select for its review screen (a normal question with Submit / Cancel).
    onSubmitMulti: () => void run(() => key("\u001b[C")),
    // "Type something": select that row, type, then Enter.
    onType: (n, t) => run(async () => { await key(String(n)); await sleep(500); await key(t); await sleep(250); await key("\r") }),
  }
  const last = rows.at(-1)
  // A question already in the transcript but unanswered shows as a card; the screen copy would duplicate it.
  const pendingRow = last?.kind === "tool" && isQuestion(last) && last.output === undefined
  const waiting = feed?.status === "waiting" && !pendingRow && !feed.ask
  const model = currentModel(items)
  // Switch by the harness's own /model command; the agent's new model shows with its next reply.
  async function chooseModel(alias?: string) {
    setError("")
    try {
      await paneInput(pane, "send", alias ? `/model ${alias}` : "/model")
      if (!alias) onOpenTerminal()
    } catch (e) { setError((e as Error).message) }
  }
  const status = statusLabel(feed?.status, tool)
  const renderRow = (r: Row) => r.kind === "msg" ? <Message key={r.item.id} item={r.item} /> : (isQuestion(r)
    ? <AskCard key={r.call.id} ask={toAsk(r)!} answer={r.output === undefined ? undefined : cleanAnswer(r.output)} actions={r.output === undefined ? actions : undefined} picked={r.output === undefined ? picked : undefined} />
    : <ToolCard key={r.call.id} row={r} />)
  return <div className="flex min-h-0 flex-1 flex-col">
    <div className="flex shrink-0 items-center gap-2 border-b px-3 py-1">
      <p role="status" className="flex min-w-0 items-center gap-1.5 truncate text-xs text-muted-foreground">
        {feed?.status === "running" ? <Loader2 className="size-3 shrink-0 animate-spin" /> : <span className={`size-1.5 shrink-0 rounded-full ${feed?.status === "waiting" ? "bg-amber-500" : feed?.status === "error" ? "bg-destructive" : "bg-emerald-500"}`} />}
        <span className="truncate">{status}</span>
      </p>
      {!!feed?.queued.length && <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground" title={feed.queued.join("\n")}>{feed.queued.length} queued</span>}
      {model && <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="xs" variant="ghost" aria-label={`Model: ${model}`} className="ml-auto shrink-0 font-mono text-muted-foreground"><Cpu />{model}<ChevronDown /></Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {(harnessModels[feed?.harness ?? ""] ?? []).map(m => <DropdownMenuItem key={m} onSelect={() => void chooseModel(m)}>{m}</DropdownMenuItem>)}
          <DropdownMenuItem onSelect={() => void chooseModel()}>Pick in Terminal…</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>}
    </div>
    <div ref={scroller} role="log" aria-label="Conversation" className="min-h-0 flex-1 overflow-y-auto"
      onScroll={e => { const el = e.currentTarget; atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48 }}>
      <div className={`${chatColumn} space-y-2 px-3 py-3`}>
        {feed?.unsupported && <p className="text-sm text-muted-foreground">Chat is not available for this agent. Use Terminal.</p>}
        {feed?.unreadable && <p className="text-sm text-muted-foreground">The transcript is not readable yet. Use Terminal.</p>}
        {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
        {segments(rows).map(seg => seg.length === 1 ? renderRow(seg[0]!) : <WorkGroup key={rowKey(seg[0]!)} rows={seg} />)}
        {feed?.ask && !pendingRow && <AskCard key={feed.ask.question} ask={feed.ask} actions={actions} picked={picked} />}
        {waiting && <div className="ml-8 rounded-xl border border-amber-500/40 bg-amber-500/5 p-3 text-xs">
          <p className="mb-1.5 font-medium text-amber-700 dark:text-amber-400">Approval needed. Answer it in Terminal.</p>
          <pre className="max-h-48 overflow-auto rounded-md bg-background p-2 whitespace-pre-wrap">{feed?.screen}</pre>
          <Button size="sm" variant="outline" className="mt-2 max-md:min-h-11" onClick={onOpenTerminal}><ExternalLink />Open in Terminal</Button>
        </div>}
      </div>
    </div>
    <Composer onSubmit={() => void send()} hint={touch ? undefined : "Enter to send · Shift+Enter for a new line · / for commands"}
      above={<>{!!commands.length && <ul id="agent-commands" role="listbox" aria-label="Commands" className="absolute inset-x-3 bottom-full z-10 mb-1 max-h-64 overflow-y-auto rounded-xl border bg-popover p-1 shadow-lg">
        {commands.map(([c, d], i) => <li key={c} role="option" aria-selected={i === pick % commands.length}>
          <button type="button" className={`flex w-full items-baseline gap-3 rounded-lg px-2.5 py-1.5 text-left text-xs ${i === pick % commands.length ? "bg-accent" : "hover:bg-accent/60"}`}
            onClick={() => setDraft(c)}><span className="font-mono">{c}</span><span className="truncate text-muted-foreground">{d}</span></button>
        </li>)}
      </ul>}{(!!images.length || uploading > 0) && <div className="mb-1 flex flex-wrap gap-1.5">
        {images.map(i => <span key={i.name} className="relative size-14 overflow-hidden rounded-lg border bg-muted">
          <img src={i.url} alt="Attached" className="size-full object-cover" />
          <button type="button" aria-label="Remove image" className="absolute right-0.5 top-0.5 flex size-5 items-center justify-center rounded-full bg-background/90 shadow max-md:size-6" onClick={() => setImages(prev => prev.filter(x => x !== i))}><X className="size-3" /></button>
        </span>)}
        {uploading > 0 && <span role="status" aria-label="Uploading" className="flex size-14 items-center justify-center rounded-lg border bg-muted/50"><Loader2 className="size-4 animate-spin text-muted-foreground" /></span>}
      </div>}{!!feed?.queued.length && <div className="mb-1.5 rounded-lg border bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
        <p>Queued until the agent is idle ({feed.queued.length}):</p>
        <ul className="mt-0.5 space-y-0.5 text-foreground">{feed.queued.map((q, i) => <li key={i} className="truncate">{q}</li>)}</ul>
      </div>}</>}>
      <input ref={fileInput} type="file" accept="image/*" multiple hidden aria-label="Attach images" onChange={e => { void attach(Array.from(e.target.files ?? [])); e.target.value = "" }} />
      <Button type="button" variant="ghost" size="icon" aria-label="Attach image" title="Attach image" className="size-7 shrink-0 self-end rounded-full text-muted-foreground max-md:size-9" onClick={() => fileInput.current?.click()}><ImagePlus /></Button>
      <textarea aria-label="Message agent" rows={1} value={draft} placeholder="Message this agent"
        onPaste={e => { const pics = Array.from(e.clipboardData.files).filter(f => f.type.startsWith("image/")); if (pics.length) { e.preventDefault(); void attach(pics) } }}
        onDrop={e => { const pics = Array.from(e.dataTransfer.files).filter(f => f.type.startsWith("image/")); if (pics.length) { e.preventDefault(); void attach(pics) } }}
        onDragOver={e => { if (e.dataTransfer.types.includes("Files")) e.preventDefault() }}
        aria-autocomplete="list" aria-controls="agent-commands"
        onChange={e => { setDraft(e.target.value); setPick(0) }} onKeyDown={onKeyDown} className={composerInput} />
      <Button type="submit" size="icon" aria-label="Send" className={sendButton} disabled={(!draft.trim() && !images.length) || sending || uploading > 0}><ArrowUp /></Button>
    </Composer>
  </div>
}

// Claude words the result differently each time ("Your questions have been answered: ..." / "The user answered: ...
// Read the answers carefully"); the answers themselves are the "question"="answer" pairs.
export function cleanAnswer(out: string) {
  const answers = [...out.matchAll(/"[^"]*"="([^"]*)"/g)].map(m => m[1]!.trim()).filter(Boolean)
  return answers.length ? answers.join(" · ") : out
}

function statusLabel(status: string | undefined, tool: string | undefined) {
  switch (status) {
    case "running": return tool ? `Working · ${tool}` : "Working"
    case "background": return "Working in background"
    case "waiting": return "Waiting for you"
    case "idle": case "done": return "Idle"
    case "error": return "Error"
    default: return status ?? ""
  }
}

function Message({ item }: { item: Item }) {
  if (item.role === "thinking") return <Fold icon={<Brain className="size-3.5 shrink-0" />} title="Reasoning">
    <p className="whitespace-pre-wrap text-muted-foreground">{item.text}</p>
  </Fold>
  if (item.role !== "user") return <Bubble who="assistant"><Markup text={item.text ?? ""} /></Bubble>
  // Uploaded images travel as file paths; show them as thumbnails.
  const { text, images } = splitImages(item.text ?? "")
  return <Bubble who="user">
    {!!images.length && <span className="mb-1 flex flex-wrap gap-1.5">{images.map(u => <a key={u} href={u} target="_blank" rel="noreferrer"><img src={u} alt="Attached" className="max-h-40 max-w-full rounded-lg border object-cover" /></a>)}</span>}
    {text && <span className="whitespace-pre-wrap">{text}</span>}
  </Bubble>
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

type ToolRow = Extract<Row, { kind: "tool" }>
interface Ask { question: string; header?: string; multiSelect?: boolean; options?: { label: string; description?: string }[] }

function questions(row: ToolRow): Ask[] | undefined {
  if (row.call.tool?.name !== "AskUserQuestion") return undefined
  try { return (JSON.parse(row.call.tool.input ?? "") as { questions?: Ask[] }).questions } catch { return undefined }
}
const isQuestion = (row: ToolRow) => !!questions(row)?.length

// AskCard: a question the agent asks, with its options as large buttons. Pending single-choice questions
// are answered with the option's number key; multi-select and free text are answered in Terminal.
interface AskView { header?: string; question: string; summary?: string; options: { n: number; label: string; description?: string; checked?: boolean }[]; multi?: boolean; other?: boolean; otherN?: number }
interface AskActions { onPick: (n: number) => void; onSubmitMulti: () => void; onType: (n: number, text: string) => Promise<void>; onOpenTerminal: () => void }
function AskCard({ ask, answer, actions, picked }: { ask: AskView; answer?: string; actions?: AskActions; picked?: number }) {
  const open = answer === undefined
  const [expanded, setExpanded] = useState(false)
  const [text, setText] = useState("")
  const [typing, setTyping] = useState(false)
  // Once answered the card is one line: the question and what was chosen. Tap to see the options again.
  if (!open && !expanded) return <button type="button" onClick={() => setExpanded(true)} aria-expanded={false} aria-label={`Question: ${ask.question}`}
    className="ml-8 flex w-[calc(100%-2rem)] items-center gap-2 rounded-xl border bg-card px-3 py-2 text-left text-[13px] hover:bg-accent/50 max-md:min-h-11">
    <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/15 text-emerald-600"><Check className="size-3.5" /></span>
    <span className="min-w-0 flex-1 truncate"><span className="text-muted-foreground">{ask.question}</span> <span className="font-medium">{answer || "(no answer)"}</span></span>
    <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
  </button>
  const live = open && !!actions
  async function typeAnswer() {
    const t = text.trim()
    if (!t || !actions || !ask.otherN || typing) return
    setTyping(true)
    try { await actions.onType(ask.otherN, t); setText("") } finally { setTyping(false) }
  }
  return <section aria-label={`Question: ${ask.question}`} className={`ml-8 overflow-hidden rounded-2xl border shadow-sm ${open ? "border-sky-500/50 bg-card ring-2 ring-sky-500/15" : "bg-card"}`}>
    <header className="flex items-start gap-2.5 px-3.5 pb-2 pt-3">
      <span className={`mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full ${open ? "bg-sky-500/15 text-sky-600" : "bg-muted text-muted-foreground"}`}><MessageCircleQuestion className="size-4" /></span>
      <div className="min-w-0">
        {ask.header && <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{ask.header}</p>}
        <p className="text-[14px] font-medium leading-snug">{ask.question}</p>
        {ask.summary && <p className="mt-0.5 text-xs text-muted-foreground">{ask.summary}</p>}
        {live && ask.multi && <p className="mt-0.5 text-xs text-muted-foreground">Tick any, then review and submit.</p>}
      </div>
    </header>
    <ol className="space-y-1.5 px-3 pb-3">{ask.options.map(o => {
      const chosen = picked === o.n
      const mark = ask.multi
        ? <span className={`flex size-6 shrink-0 items-center justify-center rounded-md border ${o.checked ? "border-sky-600 bg-sky-600 text-white" : "bg-muted/60"}`}>{o.checked && <Check className="size-4" />}</span>
        : <span className={`flex size-6 shrink-0 items-center justify-center rounded-full border text-xs font-medium tabular-nums ${chosen ? "border-sky-600 bg-sky-600 text-white" : "bg-muted/60 text-muted-foreground"}`}>{chosen ? <Loader2 className="size-3.5 animate-spin" /> : o.n}</span>
      const body = <>{mark}<span className="min-w-0"><span className="block text-[13.5px] font-medium">{o.label}</span>{o.description && <span className="block text-xs leading-snug text-muted-foreground">{o.description}</span>}</span></>
      return <li key={o.n}>{live
        ? <button type="button" role={ask.multi ? "checkbox" : undefined} aria-checked={ask.multi ? !!o.checked : undefined} disabled={!ask.multi && picked !== undefined} onClick={() => actions!.onPick(o.n)}
            className={`flex w-full items-center gap-3 rounded-xl border px-3 py-2.5 text-left transition-colors max-md:min-h-12 ${chosen || o.checked ? "border-sky-600 bg-sky-500/10" : "bg-background hover:border-sky-500/60 hover:bg-sky-500/5 active:bg-sky-500/10"} disabled:cursor-default`}>{body}</button>
        : <div className="flex items-center gap-3 rounded-xl border bg-muted/30 px-3 py-2">{body}</div>}</li>
    })}</ol>
    {live && ask.multi && <div className="px-3 pb-3"><Button className="w-full max-md:min-h-11" onClick={actions!.onSubmitMulti}>Review &amp; submit<ArrowRight /></Button></div>}
    {live && !!ask.otherN && <form className="flex gap-2 border-t bg-muted/30 px-3 py-2.5" onSubmit={e => { e.preventDefault(); void typeAnswer() }}>
      <input aria-label="Type your own answer" value={text} onChange={e => setText(e.target.value)} placeholder="Or type your own answer" disabled={typing}
        className="min-w-0 flex-1 rounded-lg border bg-background px-3 py-2 text-[13.5px] outline-none focus-visible:ring-2 focus-visible:ring-sky-500/40 max-md:min-h-11" />
      <Button type="submit" size="icon" aria-label="Send answer" disabled={!text.trim() || typing} className="max-md:size-11">{typing ? <Loader2 className="animate-spin" /> : <ArrowUp />}</Button>
    </form>}
    {live && ask.other && !ask.otherN && <footer className="border-t bg-muted/30 px-3 py-2"><Button size="sm" variant="ghost" className="max-md:min-h-11" onClick={actions!.onOpenTerminal}><ExternalLink />Answer another way in Terminal</Button></footer>}
    {!open && <footer className="flex items-start gap-2 border-t bg-muted/30 px-3.5 py-2 text-xs text-muted-foreground"><Check className="mt-0.5 size-3.5 shrink-0 text-emerald-600" /><span className="min-w-0 flex-1 whitespace-pre-wrap">{answer || "(no answer)"}</span><button type="button" className="shrink-0 underline" onClick={() => setExpanded(false)}>Collapse</button></footer>}
  </section>
}

function toAsk(row: ToolRow): AskView | undefined {
  const q = questions(row)?.[0]
  if (!q) return undefined
  return { header: q.header, question: q.question, multi: q.multiSelect, options: (q.options ?? []).map((o, i) => ({ n: i + 1, label: o.label, description: o.description })) }
}

// ToolCard: one line; tap to see the input and the output.
function ToolCard({ row }: { row: ToolRow }) {
  const tool = row.call.tool ?? {}
  return <Fold icon={toolIcon(tool.name)} title={tool.label}
    end={row.output === undefined ? <Loader2 className="size-3 animate-spin" aria-label="running" /> : <Check className="size-3 text-emerald-600" />}>
    {tool.input && tool.input !== "{}" && <pre className="max-h-40">{tool.input}</pre>}
    {row.output !== undefined && <pre className="border-t pt-1.5">{row.output || "(no output)"}</pre>}
  </Fold>
}

// Work: tool calls and reasoning with no message between them fold into one line.
const isWork = (r: Row) => r.kind === "tool" ? !isQuestion(r) : r.item.role === "thinking"
const rowKey = (r: Row) => r.kind === "msg" ? r.item.id : r.call.id
function segments(rows: Row[]): Row[][] {
  const out: Row[][] = []
  for (const r of rows) {
    const last = out[out.length - 1]
    if (last && isWork(r) && isWork(last[0]!)) last.push(r)
    else out.push([r])
  }
  return out
}

// WorkGroup: "Bash ×3 · Read ×2"; open it to see each call.
function WorkGroup({ rows }: { rows: Row[] }) {
  const calls = rows.filter((r): r is ToolRow => r.kind === "tool")
  const counts = new Map<string, number>()
  for (const c of calls) { const n = c.call.tool?.name ?? "tool"; counts.set(n, (counts.get(n) ?? 0) + 1) }
  const running = calls.some(c => c.output === undefined)
  const title = [...counts].map(([n, k]) => k > 1 ? `${n} ×${k}` : n).join(" · ") || "Reasoning"
  return <Fold icon={<Wrench className="size-3.5 shrink-0" />} title={title}
    end={running ? <Loader2 className="size-3 animate-spin" aria-label="running" /> : <Check className="size-3 text-emerald-600" />}>
    <div className="-mx-1 space-y-1.5">{rows.map(r => r.kind === "msg" ? <Message key={r.item.id} item={r.item} /> : <ToolCard key={r.call.id} row={r} />)}</div>
  </Fold>
}
