import { useLayoutEffect, useEffect, useRef, useState, type KeyboardEvent, type ReactNode } from "react"
import { ArrowRight, ArrowUp, Brain, Bot, ImageIcon, ListTodo, Paperclip, Save, Workflow, X, Check, ChevronDown, Cpu, Gauge, ExternalLink, FileText, MessageCircleQuestion, Loader2, Pencil, Search, Terminal, Wrench } from "lucide-react"
import { api, post } from "@/api"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Bubble, Composer, Fold, Markup, Typing, chatColumn, chatIndent, composerInput, sendButton } from "@/components/ChatParts"
import { paneInput } from "@/lib/paneInput"
import { clock, createdTask, currentModel, runningTool, splitSystem, toRows, type Feed, type Item, type Row } from "@/lib/transcript"
import { cn } from "@/lib/utils"
import { effortLevels } from "@/lib/roles"
import { commandMatches } from "@/lib/harnessCommands"
import { useAppState } from "@/state"
import { applyMention, filterMentions, mentionItems, mentionQuery, type Mention } from "@/lib/mentions"
import type { Project } from "@/types"
import { humanSize, splitAttachments, uploadFile, type Upload } from "@/lib/uploads"

const MAX_ITEMS = 1000
const POLL_MS = 1500
const LIVE_MS = 1000 // while the agent is running, so new output shows within about a second

// AgentChat renders the agent's harness transcript. The agent keeps running in tmux; this only reads it and sends text.
// onSave, when given, offers each message a "Save to ticket" button (a ticket's Chat tab only).
export function AgentChat({ pane, onOpenTerminal, onUnavailable, project, onSave }: { pane: string; onOpenTerminal: () => void; onUnavailable: () => void; project?: Project; onSave?: (text: string) => void }) {
  const [items, setItems] = useState<Item[]>([])
  const [feed, setFeed] = useState<Feed>()
  const [draft, setDraft] = useState("")
  const [sending, setSending] = useState(false)
  const [error, setError] = useState("")
  const [pick, setPick] = useState(0)
  const [picked, setPicked] = useState<number>()
  const [files, setFiles] = useState<Upload[]>([])
  const [uploading, setUploading] = useState(0)
  const fileInput = useRef<HTMLInputElement>(null)
  const photoInput = useRef<HTMLInputElement>(null)
  const box = useRef<HTMLTextAreaElement>(null)
  const [caret, setCaret] = useState(0)
  const [mentionPick, setMentionPick] = useState(0)
  const [mentionOff, setMentionOff] = useState(false)
  const { state } = useAppState()
  const cursor = useRef("")
  const kick = useRef(() => {}) // poll again soon, e.g. right after a key was sent
  const seen = useRef("") // the last feed view shown; cleared when the view is changed locally, so the next poll repaints
  const unavailable = useRef(onUnavailable)
  useEffect(() => { unavailable.current = onUnavailable })
  const scroller = useRef<HTMLDivElement>(null)
  const atBottom = useRef(true)
  const touch = typeof matchMedia === "function" && matchMedia("(pointer: coarse)").matches

  // Poll while visible: one timer chain, never two requests at once. A kick during a request only moves the next poll.
  useEffect(() => {
    let stopped = false
    let inFlight = false
    let timer = 0
    let kicked = false
    const schedule = (ms: number) => { clearTimeout(timer); if (!stopped) timer = window.setTimeout(() => void tick(), ms) }
    const tick = async (): Promise<void> => {
      if (inFlight) return
      let delay = POLL_MS
      if (document.visibilityState === "visible") {
        inFlight = true
        try {
          const after = cursor.current ? `?after=${encodeURIComponent(cursor.current)}` : ""
          const f = await api<Feed>(`/api/agents/${encodeURIComponent(pane)}/transcript${after}`)
          if (stopped) return
          setError("")
          // Unreadable is not final: a new agent has no transcript until its first message.
          if (f.unsupported) unavailable.current()
          if (f.reset || f.cursor) cursor.current = f.cursor
          if (f.reset) setItems(f.items)
          else if (f.items.length) setItems(prev => [...prev, ...f.items].slice(-MAX_ITEMS))
          // Skip the re-render when nothing changed, so an idle thread does not flicker.
          const view: Feed = { ...f, items: [], cursor: "", reset: false, more: false }
          const sig = JSON.stringify(view)
          if (sig !== seen.current) { seen.current = sig; setFeed(view) }
          delay = f.more ? 0 : f.status === "running" || f.status === "background" ? LIVE_MS : POLL_MS
        } catch (e) {
          if (!stopped) setError((e as Error).message)
        } finally {
          inFlight = false
        }
      }
      // A kick that came during this request still gets its quick poll.
      schedule(kicked ? 400 : delay)
      kicked = false
    }
    void tick()
    kick.current = () => { if (inFlight) kicked = true; else schedule(400) }
    return () => { stopped = true; clearTimeout(timer); kick.current = () => {} }
  }, [pane])

  // Follow new messages only while the reader is at the bottom.
  useLayoutEffect(() => {
    const el = scroller.current
    if (el && atBottom.current) el.scrollTop = el.scrollHeight
  }, [items, feed])

  // The question is gone from the screen once it is answered: forget the choice so the next one is clickable.
  const askKey = feed?.ask ? feed.ask.question + "|" + feed.ask.options.map(o => o.label).join(",") : ""
  useEffect(() => { setPicked(undefined) }, [askKey])

  const rows = toRows(items)
  const tool = runningTool(rows)

  async function send() {
    // Attached files go in as their paths, one per line, which is how an agent opens them.
    const text = [draft.trim(), ...files.map(f => f.path)].filter(Boolean).join("\n")
    if (!text || sending || uploading) return
    setSending(true)
    setError("")
    try {
      const r = (await paneInput(pane, "send", text)) as { queued: boolean }
      setDraft("")
      setFiles([])
      atBottom.current = true
      if (r.queued) { setFeed(f => f && { ...f, queued: [...f.queued, text] }); seen.current = "" }
      kick.current()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSending(false)
    }
  }

  async function attach(picked: File[]) {
    if (!picked.length) return
    setError("")
    setUploading(n => n + picked.length)
    for (const f of picked) {
      try { const u = await uploadFile(f); setFiles(prev => [...prev, u]) } catch (e) { setError((e as Error).message) } finally { setUploading(n => n - 1) }
    }
  }

  // `@` tags an agent, a ticket or a workflow; `#` tags a ticket. The chosen name goes into the message.
  const mq = mentionOff ? null : mentionQuery(draft, caret)
  const mentions = mq ? filterMentions(mentionItems(state?.agents ?? [], project?.tasks ?? [], project?.workflows ?? [], pane), mq) : []
  function chooseMention(m: Mention) {
    if (!mq) return
    const next = applyMention(draft, caret, mq.start, m)
    setDraft(next.text)
    setCaret(next.caret)
    setMentionPick(0)
    requestAnimationFrame(() => { box.current?.focus(); box.current?.setSelectionRange(next.caret, next.caret) })
  }

  // Enter sends on desktop; Shift+Enter and IME composition keep their normal meaning.
  const commands = commandMatches(feed?.harness, draft)
  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (mentions.length && !e.nativeEvent.isComposing) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        e.preventDefault()
        setMentionPick(p => (p + (e.key === "ArrowDown" ? 1 : mentions.length - 1)) % mentions.length)
        return
      }
      if (e.key === "Escape") { e.preventDefault(); setMentionOff(true); return }
      if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey)) { e.preventDefault(); chooseMention(mentions[mentionPick % mentions.length]!); return }
    }
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
  const seenModel = currentModel(items)
  // Show a pick at once; the transcript only reports the model with the next reply, so drop the pick when that changes.
  const [pickedModel, setPickedModel] = useState<{ pane: string; from?: string; value: string }>()
  const [effortSet, setEffortSet] = useState<{ pane: string; level: string }>()
  const model = pickedModel?.pane === pane && pickedModel.from === seenModel ? pickedModel.value : seenModel
  // Switch by the harness's own /model command; the agent's new model shows with its next reply.
  async function chooseModel(alias?: string) {
    setError("")
    try {
      await paneInput(pane, "send", alias ? `/model ${alias}` : "/model")
      if (alias) setPickedModel({ pane, from: seenModel, value: alias })
      if (!alias) onOpenTerminal()
    } catch (e) { setError((e as Error).message) }
  }
  // Only claude is known to take "/effort <level>"; pi and codex have no verified in-chat command.
  const effort = feed?.harness === "claude" ? effortLevels("claude") : []
  async function chooseEffort(level: string) {
    setError("")
    try { await paneInput(pane, "send", `/effort ${level}`); setEffortSet({ pane, level }) } catch (e) { setError((e as Error).message) }
  }
  const status = statusLabel(feed?.status, tool)
  const busy = feed?.status === "running" || feed?.status === "background"
  // Work rows (tool calls, reasoning) fold into WorkGroup; a message or a question is rendered as it is.
  const renderRow = (r: Row) => r.kind === "msg" ? <Message key={r.item.id} item={r.item} onSave={onSave} /> : createdTask(r) ? <TaskCard key={r.call.id} row={r} project={project} /> : <AskCard key={r.call.id} ask={toAsk(r)!} answer={r.output === undefined ? undefined : cleanAnswer(r.output)} actions={r.output === undefined ? actions : undefined} picked={r.output === undefined ? picked : undefined} />
  const renderSeg = (seg: Row[]) => isWork(seg[0]!) ? <WorkGroup key={rowKey(seg[0]!)} rows={seg} /> : renderRow(seg[0]!)
  return <div className="flex min-h-0 flex-1 flex-col">
    <div className="flex shrink-0 items-center gap-2 border-b px-3 py-1">
      <p role="status" className="flex min-w-0 items-center gap-1.5 truncate text-xs text-muted-foreground">
        {feed?.status === "running" ? <Loader2 className="size-3 shrink-0 animate-spin" /> : <span className={`size-1.5 shrink-0 rounded-full ${feed?.status === "waiting" ? "bg-amber-500" : feed?.status === "error" ? "bg-destructive" : "bg-emerald-500"}`} />}
        <span className="truncate">{status}</span>
      </p>
      {!!feed?.queued.length && <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground" title={feed.queued.join("\n")}>{feed.queued.length} queued</span>}
      {effort.length > 0 && <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="xs" variant="ghost" aria-label="Effort" className="ml-auto shrink-0 text-muted-foreground"><Gauge />{effortSet?.pane === pane ? effortSet.level : "effort"}<ChevronDown /></Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {effort.map(l => <DropdownMenuItem key={l} onSelect={() => void chooseEffort(l)}>{l}</DropdownMenuItem>)}
        </DropdownMenuContent>
      </DropdownMenu>}
      {model && <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="xs" variant="ghost" aria-label={`Model: ${model}`} className={`${effort.length ? "" : "ml-auto "}min-w-0 shrink-0 font-mono text-muted-foreground max-sm:max-w-[40%]`}><Cpu /><span className="truncate">{model}</span><ChevronDown /></Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {(state?.models?.[feed?.harness ?? ""] ?? []).map(m => <DropdownMenuItem key={m} onSelect={() => void chooseModel(m)}>{m}</DropdownMenuItem>)}
          <DropdownMenuItem onSelect={() => void chooseModel()}>Pick in Terminal…</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>}
    </div>
    <div ref={scroller} role="log" aria-label="Conversation" className="min-h-0 flex-1 overflow-y-auto"
      onScroll={e => { const el = e.currentTarget; atBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48 }}>
      <div className={`${chatColumn} space-y-2 px-3 py-3`}>
        {feed?.unsupported && <p className="text-sm text-muted-foreground">Chat is not available for this agent. Use Terminal.</p>}
        {feed?.unreadable && !items.length && <p className="text-sm text-muted-foreground">The transcript is not readable yet. Use Terminal.</p>}
        {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
        {segments(rows).map(renderSeg)}
        {feed?.ask && !pendingRow && <AskCard key={feed.ask.question} ask={feed.ask} actions={actions} picked={picked} />}
        {waiting && <div className={cn(chatIndent, "rounded-xl border border-amber-500/40 bg-amber-500/5 p-3 text-xs")}>
          <p className="mb-1.5 font-medium text-amber-700 dark:text-amber-400">Approval needed. Answer it in Terminal.</p>
          <pre className="max-h-48 overflow-auto rounded-md bg-background p-2 whitespace-pre-wrap">{feed?.screen}</pre>
          <Button size="sm" variant="outline" className="mt-2 max-md:min-h-11" onClick={onOpenTerminal}><ExternalLink />Open in Terminal</Button>
        </div>}
        {busy && <Typing label={status} />}
      </div>
    </div>
    <Composer onSubmit={() => void send()} hint={touch ? undefined : "Enter to send · Shift+Enter for a new line · / for commands"}
      above={<>{!!mentions.length && <ul id="agent-mentions" role="listbox" aria-label="Mentions" className="absolute inset-x-3 bottom-full z-10 mb-1 max-h-64 overflow-y-auto rounded-xl border bg-popover p-1 shadow-lg">
        {mentions.map((m, i) => <li key={m.kind + m.key} role="option" aria-selected={i === mentionPick % mentions.length}>
          <button type="button" className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-left text-xs max-md:min-h-11 ${i === mentionPick % mentions.length ? "bg-accent" : "hover:bg-accent/60"}`}
            onMouseDown={e => e.preventDefault()} onClick={() => chooseMention(m)}>
            <MentionIcon kind={m.kind} />
            <span className="min-w-0 flex-1 truncate font-medium">{m.label}</span>
            <span className="max-w-[45%] shrink-0 truncate text-muted-foreground">{m.detail}</span>
          </button>
        </li>)}
      </ul>}{!!commands.length && <ul id="agent-commands" role="listbox" aria-label="Commands" className="absolute inset-x-3 bottom-full z-10 mb-1 max-h-64 overflow-y-auto rounded-xl border bg-popover p-1 shadow-lg">
        {commands.map(([c, d], i) => <li key={c} role="option" aria-selected={i === pick % commands.length}>
          <button type="button" className={`flex w-full items-baseline gap-3 rounded-lg px-2.5 py-1.5 text-left text-xs ${i === pick % commands.length ? "bg-accent" : "hover:bg-accent/60"}`}
            onClick={() => setDraft(c)}><span className="font-mono">{c}</span><span className="truncate text-muted-foreground">{d}</span></button>
        </li>)}
      </ul>}{(!!files.length || uploading > 0) && <div className="mb-1 flex flex-wrap gap-1.5">
        {files.map(f => f.image
          ? <span key={f.url} className="relative size-14 overflow-hidden rounded-lg border bg-muted">
              <img src={f.url} alt="Attached" className="size-full object-cover" />
              <button type="button" aria-label={`Remove ${f.name}`} className="absolute right-0.5 top-0.5 flex size-5 items-center justify-center rounded-full bg-background/90 shadow max-md:size-6" onClick={() => setFiles(prev => prev.filter(x => x !== f))}><X className="size-3" /></button>
            </span>
          : <span key={f.url} className="flex h-14 max-w-56 items-center gap-2 rounded-lg border bg-muted/50 pl-2.5 pr-1.5">
              <FileText className="size-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0"><span className="block truncate text-xs font-medium">{f.name}</span><span className="block text-[11px] text-muted-foreground">{humanSize(f.size)}</span></span>
              <button type="button" aria-label={`Remove ${f.name}`} className="flex size-6 shrink-0 items-center justify-center rounded-full hover:bg-accent" onClick={() => setFiles(prev => prev.filter(x => x !== f))}><X className="size-3" /></button>
            </span>)}
        {uploading > 0 && <span role="status" aria-label="Uploading" className="flex size-14 items-center justify-center rounded-lg border bg-muted/50"><Loader2 className="size-4 animate-spin text-muted-foreground" /></span>}
      </div>}{!!feed?.queued.length && <div className="mb-1.5 rounded-lg border bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
        <p>Queued until the agent is idle ({feed.queued.length}):</p>
        <ul className="mt-0.5 space-y-0.5 text-foreground">{feed.queued.map((q, i) => <li key={i} className="truncate">{q}</li>)}</ul>
      </div>}</>}>
      <input ref={fileInput} type="file" multiple hidden aria-label="Attach files" onChange={e => { void attach(Array.from(e.target.files ?? [])); e.target.value = "" }} />
      {/* accept="image/*" makes phones offer the photo gallery and camera, which a plain file picker hides on Android. */}
      <input ref={photoInput} type="file" accept="image/*" multiple hidden aria-label="Attach photos" onChange={e => { void attach(Array.from(e.target.files ?? [])); e.target.value = "" }} />
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button type="button" variant="ghost" size="icon" aria-label="Attach" title="Attach a file or photo" className="size-7 shrink-0 self-end rounded-full text-muted-foreground max-md:size-9"><Paperclip /></Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem onSelect={() => photoInput.current?.click()}><ImageIcon />Photo</DropdownMenuItem>
          <DropdownMenuItem onSelect={() => fileInput.current?.click()}><FileText />File</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <textarea ref={box} aria-label="Message agent" rows={1} value={draft} placeholder="Message this agent"
        onSelect={e => setCaret(e.currentTarget.selectionStart)}
        onPaste={e => { const picked = Array.from(e.clipboardData.files); if (picked.length) { e.preventDefault(); void attach(picked) } }}
        onDrop={e => { const picked = Array.from(e.dataTransfer.files); if (picked.length) { e.preventDefault(); void attach(picked) } }}
        onDragOver={e => { if (e.dataTransfer.types.includes("Files")) e.preventDefault() }}
        aria-autocomplete="list" aria-controls={mentions.length ? "agent-mentions" : "agent-commands"}
        onChange={e => { setDraft(e.target.value); setCaret(e.target.selectionStart); setPick(0); setMentionPick(0); setMentionOff(false) }} onKeyDown={onKeyDown} className={composerInput} />
      <Button type="submit" size="icon" aria-label="Send" className={sendButton} disabled={(!draft.trim() && !files.length) || sending || uploading > 0}><ArrowUp /></Button>
    </Composer>
  </div>
}

// Claude words the result differently each time ("Your questions have been answered: ..." / "The user answered: ...
// Read the answers carefully"); the answers themselves are the "question"="answer" pairs.
export function cleanAnswer(out: string) {
  const answers = [...out.matchAll(/"[^"]*"="([^"]*)"/g)].map(m => m[1]!.trim()).filter(Boolean)
  return answers.length ? answers.join(" · ") : out
}

function MentionIcon({ kind }: { kind: Mention["kind"] }) {
  const cls = "size-4 shrink-0 text-muted-foreground"
  return kind === "agent" ? <Bot className={cls} /> : kind === "task" ? <ListTodo className={cls} /> : <Workflow className={cls} />
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

// SaveToTicket is the human's explicit promotion of a chat message to a ticket
// comment: chat is ephemeral, the ticket's activity is not.
function SaveToTicket({ text, onSave, align }: { text: string; onSave?: (text: string) => void; align: "start" | "end" }) {
  if (!onSave || !text.trim()) return null
  return <div className={`mt-0.5 flex ${align === "end" ? "justify-end" : `${chatIndent} justify-start`}`}>
    <button type="button" onClick={() => onSave(text)} title="Post this message as a comment on the ticket"
      className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-muted-foreground opacity-0 transition-opacity hover:text-foreground focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-1 group-hover/msg:opacity-100 max-md:opacity-100">
      <Save className="size-3" />Save to ticket
    </button>
  </div>
}

// Stamp: when a message was written. Tool rows have none.
function Stamp({ iso, end }: { iso?: string; end?: boolean }) {
  const t = clock(iso)
  return t ? <time dateTime={iso} title={new Date(iso!).toLocaleString()} className={cn("mt-0.5 block text-[11px] text-muted-foreground", end ? "text-right" : "ml-[34px] max-sm:ml-0")}>{t}</time> : null
}

function Message({ item, onSave }: { item: Item; onSave?: (text: string) => void }) {
  if (item.role !== "user") return <div className="group/msg">
    <Bubble who="assistant"><Markup text={item.text ?? ""} /></Bubble>
    <Stamp iso={item.time} />
    <SaveToTicket text={item.text ?? ""} onSave={onSave} align="start" />
  </div>
  // The harness's compaction summary arrives as a user message: fold it instead of showing a wall of text.
  if (/^This session is being continued from a previous conversation/.test((item.text ?? "").trim())) return <Fold icon={<ListTodo className="size-3.5 shrink-0" />} title="Conversation summary">
    <Markup text={item.text ?? ""} />
  </Fold>
  // Uploaded files travel as paths; show images as thumbnails and other files as download cards.
  const sys = splitSystem(item.text ?? "")
  const { text, files } = splitAttachments(sys.text)
  const notes = sys.notes.map((n, i) => <p key={i} className="px-2 py-0.5 text-center text-[11px] text-muted-foreground">{n}</p>)
  if (!text && !files.length) return <>{notes}</>
  return <div className="group/msg">{notes}<Bubble who="user">
    {!!files.length && <span className="mb-1 flex flex-wrap gap-1.5">{files.map(f => f.image
      ? <a key={f.url} href={f.url} target="_blank" rel="noreferrer"><img src={f.url} alt={f.name} className="max-h-40 max-w-full rounded-lg border object-cover" /></a>
      : <a key={f.url} href={f.url} download={f.name} className="flex max-w-full items-center gap-2 rounded-lg border bg-background px-2.5 py-1.5 text-xs hover:bg-accent"><FileText className="size-4 shrink-0" /><span className="truncate font-medium">{f.name}</span></a>)}</span>}
    {text && <span className="whitespace-pre-wrap">{text}</span>}
  </Bubble>
  <Stamp iso={item.time} end />
  <SaveToTicket text={text} onSave={onSave} align="end" />
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
    className={`${chatIndent} flex w-[calc(100%-2rem)] items-center gap-2 rounded-xl border bg-card px-3 py-2 text-left text-[13px] hover:bg-accent/50 max-sm:w-full max-md:min-h-11`}>
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
  return <section aria-label={`Question: ${ask.question}`} className={`${chatIndent} overflow-hidden rounded-2xl border shadow-sm ${open ? "border-sky-500/50 bg-card ring-2 ring-sky-500/15" : "bg-card"}`}>
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

// Work: tool calls and reasoning with no message between them fold into one card.
const isWork = (r: Row) => r.kind === "tool" ? !isQuestion(r) && !createdTask(r) : r.item.role === "thinking"

// TaskCard: a ticket the agent just created, linked to its run page; title and status come from the board when it has it.
function TaskCard({ row, project }: { row: ToolRow; project?: Project }) {
  const made = createdTask(row)!
  const task = project?.tasks?.find(t => t.id === made.id)
  const open = () => window.dispatchEvent(new CustomEvent("tagent:ticket", { detail: { id: made.id, dir: project?.dir } }))
  return <div className={chatIndent}>
    <button type="button" onClick={open} className="flex w-full min-w-0 items-center gap-2 rounded-lg border bg-card px-3 py-2 text-left text-sm hover:bg-accent">
      <ListTodo className="size-4 shrink-0 text-teal-700 dark:text-teal-400" />
      <span className="shrink-0 font-mono text-xs text-muted-foreground">#{made.id}</span>
      <span className="min-w-0 truncate">{task?.title || made.title || "New task"}</span>
      {task?.status && <span className="ml-auto shrink-0 rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">{task.status}</span>}
    </button>
  </div>
}
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

const rowIcon = (r: Row): ReactNode => r.kind === "msg" ? <Brain className="size-3.5 shrink-0" /> : toolIcon(r.call.tool?.name)
const rowTitle = (r: Row) => r.kind === "msg" ? "Reasoning" : r.call.tool?.label
const rowEnd = (r: Row) => r.kind === "msg" ? undefined : r.output === undefined ? <Loader2 className="size-3 animate-spin" aria-label="running" /> : <Check className="size-3 text-emerald-600" />
function RowBody({ row }: { row: Row }) {
  if (row.kind === "msg") return <p className="whitespace-pre-wrap text-muted-foreground">{row.item.text}</p>
  const input = row.call.tool?.input
  return <>
    {input && input !== "{}" && <pre className="max-h-40">{input}</pre>}
    {row.output !== undefined && <pre className="border-t pt-1.5">{row.output || "(no output)"}</pre>}
  </>
}

// WorkGroup: one row shows its own title and details; several show "Bash ×3 · Read ×2" with each row nested.
// It is always the same Fold, so an open card stays open when a second row arrives.
function WorkGroup({ rows }: { rows: Row[] }) {
  const one = rows.length === 1 ? rows[0] : undefined
  const calls = rows.filter((r): r is ToolRow => r.kind === "tool")
  const counts = new Map<string, number>()
  for (const c of calls) { const n = c.call.tool?.name ?? "tool"; counts.set(n, (counts.get(n) ?? 0) + 1) }
  const running = calls.some(c => c.output === undefined)
  const title = one ? rowTitle(one) : [...counts].map(([n, k]) => k > 1 ? `${n} ×${k}` : n).join(" · ") || "Reasoning"
  const end = one ? rowEnd(one) : calls.length ? (running ? <Loader2 className="size-3 animate-spin" aria-label="running" /> : <Check className="size-3 text-emerald-600" />) : undefined
  return <Fold icon={one ? rowIcon(one) : <Wrench className="size-3.5 shrink-0" />} title={title} end={end}>
    {one ? <RowBody row={one} /> : <div className="-mx-1 space-y-1.5">
      {rows.map(r => <Fold key={rowKey(r)} className="ml-0" icon={rowIcon(r)} title={rowTitle(r)} end={rowEnd(r)}><RowBody row={r} /></Fold>)}
    </div>}
  </Fold>
}
