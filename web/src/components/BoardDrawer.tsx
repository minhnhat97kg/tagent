import { attachmentURL, uploadError as validateUpload } from "@/lib/attachments"
import { toast } from "sonner"
import { AttachmentRows, AttachmentViewer, type Attachment } from "./Attachments"
import { Fragment, useEffect, useRef, useState, type ReactNode } from "react"
import { DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { act, api, post } from "@/api"
import { ArrowRight, Check, ChevronDown, Paperclip, Pencil, Play, Plus, Reply, Trash2, TriangleAlert, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { RunView } from "./RunPage"
import { TicketChat } from "./TicketChat"
import { useTicketChat } from "@/hooks/useTicketChat"
import { enc } from "@/hooks/useRoute"
import { usePoll } from "@/hooks/usePoll"
import type { Money, Project, RunEntry, RunView as RunData, Task, TicketUsage, Workflow } from "@/types"

// TABS splits the ticket panel: the facts, the ticket's chat, the conversation
// on the ticket, and what actually ran. Chat is disabled with a reason when the
// ticket's pane cannot be read (see useTicketChat).
const TABS = [{ id: "details", label: "Details" }, { id: "chat", label: "Chat" }, { id: "activity", label: "Activity" }, { id: "run", label: "Run" }] as const
type TabID = (typeof TABS)[number]["id"]

// dur formats a Go duration (nanoseconds).
const dur = (ns: number) => { const s = Math.round(ns / 1e9); return s >= 3600 ? `${Math.floor(s / 3600)}h ${Math.round(s % 3600 / 60)}m` : s >= 60 ? `${Math.floor(s / 60)}m ${s % 60}s` : `${s}s` }
const tok = (n: number) => n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(1)}k` : String(n)
const usd1 = (v: number) => v > 0 && v < 0.01 ? "<$0.01" : `$${v.toFixed(2)}`
// byModel groups a ticket's jobs per model for the usage bar: tokens and summed cost, largest first.
const MODEL_COLORS = ["var(--brand)", "#0284c7", "#059669", "#d97706", "#64748b"]
const modelName = (j: { model: string; harness: string }) => (j.model || j.harness || "?").split("/").pop()!
function byModel(jobs: TicketUsage["jobs"]) {
  const m = new Map<string, { name: string; tokens: number; cost?: Money }>()
  for (const j of jobs) {
    if (!j.has) continue
    const g = m.get(modelName(j)) ?? { name: modelName(j), tokens: 0 }
    g.tokens += j.count.Total
    if (j.cost) g.cost = { low: (g.cost?.low ?? 0) + j.cost.low, high: (g.cost?.high ?? 0) + j.cost.high }
    m.set(g.name, g)
  }
  return [...m.values()].sort((a, b) => b.tokens - a.tokens).map((g, i) => ({ ...g, color: MODEL_COLORS[i % MODEL_COLORS.length]! }))
}
const usd = (m?: Money) => !m ? "" : m.low === m.high ? usd1(m.low) : `${usd1(m.low)}–${usd1(m.high)}`

// Status colours are reserved and always sit next to a text label.
const COLORS: Record<string, string> = { todo: "#a1a1aa", plan: "#3b82f6", planning: "#3b82f6", researching: "#3b82f6", reproducing: "#3b82f6", build: "#6366f1", building: "#6366f1", refactoring: "#6366f1", fixing: "#6366f1", writing: "#6366f1", test: "#0ea5e9", testing: "#0ea5e9", review: "#f59e0b", reviewing: "#f59e0b", blocked: "#ef4444", done: "#10b981" }
export const statusColor = (s: string) => COLORS[s] ?? "#a1a1aa"
export function StatusBadge({ status }: { status: string }) {
  return <span className="status-badge"><i style={{ background: statusColor(status) }} />{status}</span>
}
// shortId keeps ticket keys readable: numeric ids as #21, generated ids by their last 4 characters.
export const shortId = (id: string) => "#" + (/^\d+$/.test(id) ? id : id.slice(-4))
// Priority is an ordering rank, not a label: high first, then normal, then low.
export const priorityRank = (p?: string) => (p === "high" ? 0 : p === "low" ? 2 : 1)
export const priorityMark = (p?: string) => (p === "high" ? "↑" : p === "low" ? "↓" : "")
// needState is a card's dependency badge: "waits #12", "ready" or "" (no dependencies).
export function needState(t: Task, all: Task[]) {
  if (!t.needs?.length || t.status !== "todo") return ""
  const open = t.needs.filter(id => { const dep = all.find(x => x.id === id); return !!dep && dep.status !== "done" }) // a deleted dependency stops blocking
  return open.length ? `waits ${open.map(shortId).join(", ")}` : "ready"
}
export function rel(iso?: string) {
  const t = iso ? Date.parse(iso) : NaN
  if (Number.isNaN(t)) return iso || ""
  const s = Math.max(0, (Date.now() - t) / 1000)
  return s < 60 ? "just now" : s < 3600 ? `${Math.floor(s / 60)}m ago` : s < 86400 ? `${Math.floor(s / 3600)}h ago` : s < 604800 ? `${Math.floor(s / 86400)}d ago` : new Date(t).toLocaleDateString()
}
// Role colours: fixed for the usual roles, hashed for the rest. No violet (style brief).
const TONES = [["#dbeafe", "#1d4ed8"], ["#ccfbf1", "#0f766e"], ["#fef3c7", "#b45309"], ["#e0f2fe", "#0369a1"], ["#ffe4e6", "#be123c"], ["#dcfce7", "#15803d"]]
const FIXED: Record<string, number> = { planner: 0, implementer: 1, reviewer: 2, researcher: 3, frontend: 4 }
export function roleTone(role: string) {
  if (role === "ci" || role === "claude") return ["#f4f4f5", "#52525b"]
  const i = FIXED[role] ?? [...role].reduce((h, c) => h * 31 + c.charCodeAt(0), 7) % TONES.length
  return TONES[Math.abs(i) % TONES.length]!
}
export function Avatar({ role }: { role: string }) {
  if (!role) return null
  const [bg, fg] = roleTone(role)
  return <span title={role} aria-label={role} className="board-avatar" style={{ background: bg, color: fg }}>{role.slice(0, 2).toUpperCase()}</span>
}
export function title(t: Task) { return t.title || t.text.split("\n")[0] || "Untitled task" }

// Md renders the little markdown tickets use: blank-line blocks, "- " lists, a short first line as a heading, `code`.
function inline(text: string) {
  return text.split(/(`[^`]+`)/).map((p, i) => p.startsWith("`") && p.endsWith("`") && p.length > 2 ? <code key={i}>{p.slice(1, -1)}</code> : <Fragment key={i}>{p}</Fragment>)
}
export function Md({ text }: { text: string }) {
  const blocks = text.trim().split(/\n\s*\n/)
  return <div className="md">{blocks.map((b, i) => {
    if (b.startsWith("```")) return <pre key={i}><code>{b.replace(/^```[^\n]*\n/, "").replace(/\n```$/, "")}</code></pre>
    const lines = b.split("\n")
    const out: ReactNode[] = []
    const head = lines[0]!.replace(/^#+\s*/, "")
    if (lines.length > 1 && (/^#/.test(lines[0]!) || (head.length < 40 && !/[.:,;]$/.test(head) && !head.startsWith("- ")))) { out.push(<h4 key="h">{head}</h4>); lines.shift() }
    const items = lines.filter(l => /^\s*[-*] /.test(l))
    if (items.length === lines.length && items.length) out.push(<ul key="l">{items.map((l, j) => <li key={j}>{inline(l.replace(/^\s*[-*] /, ""))}</li>)}</ul>)
    else if (lines.length) out.push(<p key="p">{inline(lines.join("\n"))}</p>)
    return <Fragment key={i}>{out}</Fragment>
  })}</div>
}

// Activity filters: what the timeline shows.
const FILTERS = [{ id: "all", label: "All" }, { id: "comments", label: "Comments" }, { id: "history", label: "History" }] as const
type FilterID = (typeof FILTERS)[number]["id"]
const SORT_KEY = "board.activitySort"
const stamp = (iso?: string) => { const t = iso ? Date.parse(iso) : NaN; return Number.isNaN(t) ? 0 : t }
// mentions reports whether a comment body tags the role, as the board's mention trigger reads it.
const mentions = (body: string, role: string) => new RegExp(`@${role.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}(?![\\w-])`).test(body)

// History rows are the ticket's moves and dispatches, read from its run (the same data as the Run tab).
type Row = { key: string; when?: string; comment?: NonNullable<Task["comments"]>[number]; icon?: ReactNode; text?: string }
function historyRows(entries: RunEntry[]): Row[] {
  return entries.flatMap((e, i) => {
    const prev = entries[i - 1]
    const rows: Row[] = [{ key: `m${e.seq}`, when: e.start, icon: <ArrowRight />, text: prev ? `${prev.role || "system"} moved ${prev.step} → ${e.step}` : `started ${e.step}` }]
    if (e.role) rows.push({ key: `d${e.seq}`, when: e.start, icon: <Play />, text: `dispatched ${e.role}${e.harness ? ` (${e.harness}${e.model ? `, ${e.model}` : ""})` : ""}` })
    return rows
  })
}

export function BoardDrawer({ task, statuses, roles, role, reason, dir, reload, workflows, project, initialReply = '', onOpen }: { initialReply?: string; onOpen?: (id: string) => void; project?: Project; task: Task; statuses: string[]; roles: string[]; role: string; reason?: string; dir: string; reload: () => void; workflows: Workflow[] }) {
  // The ticket's chat pane: the tab needs to know before it is opened whether Chat can be shown at all.
  const chat = useTicketChat(dir, task.id)
  const chatOff = !!chat.info && !chat.info.available
  const tabAllowed = (i: number) => !(TABS[i]!.id === "chat" && chatOff)
  // Saving a chat message is the human's action: it posts a normal ticket comment, so it
  // shows in Activity, reaches the agents, and outlives the pane.
  const saveToTicket = (text: string) => {
    const body = text.trim()
    if (!body) return
    act(post<{ notice?: string }>("/api/tasks", { Dir: dir, Op: "comment", ID: task.id, Text: body }).then(r => {
      toast.success(`Saved to ${shortId(task.id)}`)
      if (r?.notice) toast(r.notice)
      reload()
    }))
  }
  const [body, setBody] = useState(initialReply)
  const [picking, setPicking] = useState(false)
  const [depFilter, setDepFilter] = useState("")
  const [usageOpen, setUsageOpen] = useState(false)
  const [sending, setSending] = useState(false)
  const [files, setFiles] = useState<File[]>([])
  const [attachments, setAttachments] = useState<Attachment[]>([])
  const [selected, setSelected] = useState<Attachment | null>(null)
  const [uploadError, setUploadError] = useState("")
  const url = attachmentURL(dir, task.id)
  useEffect(() => {
    const controller = new AbortController()
    fetch(url, { signal: controller.signal }).then(r => { if (!r.ok) throw new Error("Cannot list attachments"); return r.json() }).then(setAttachments).catch(e => { if (!controller.signal.aborted) setUploadError(String(e)) })
    return () => controller.abort()
  }, [url, task.updated])
  // History comes from the run: only a ticket with a workflow has one.
  const [history, setHistory] = useState<RunEntry[]>([])
  useEffect(() => {
    if (!task.workflow) return
    api<RunData>(`/api/tasks/${enc(task.id)}/run?dir=${enc(dir)}`).then(r => setHistory(r.entries ?? [])).catch(() => setHistory([]))
  }, [dir, task.id, task.workflow, task.updated])
  const usage = usePoll(() => api<TicketUsage>(`/api/tasks/${enc(task.id)}/usage?dir=${enc(dir)}`), task.status === "done" ? 3_600_000 : 3000, dir + task.id).data
  const enqueue = (incoming: File[]) => {
    const error = validateUpload([...files, ...incoming]); if (error) { setUploadError(error); return }
    setFiles([...files, ...incoming]); setUploadError("")
  }
  const submitComment = async () => {
    setSending(true); setUploadError("")
    try {
      const data = new FormData(); data.set("text", body); files.forEach(f => data.append("files", f))
      const r = await fetch(url, { method: "POST", body: data }); if (!r.ok) throw new Error(await r.text())
      const res: { notice?: string } = await r.json().catch(() => ({}))
      setBody(""); setFiles([]); setComposerOpen(false); reload()
      if (res.notice) toast(res.notice)
    } catch (e) { setUploadError(String(e)) } finally { setSending(false) }
  }
  const [composerOpen, setComposerOpen] = useState(!!initialReply)
  const [index, setIndex] = useState(0)
  const [cursor, setCursor] = useState(0)
  const [dismissed, setDismissed] = useState(false)
  const input = useRef<HTMLTextAreaElement>(null)
  // Opening the composer puts the caret at the end of whatever is drafted.
  useEffect(() => {
    if (!composerOpen) return
    const frame = requestAnimationFrame(() => { const el = input.current; if (!el) return; el.focus({ preventScroll: true }); el.setSelectionRange(el.value.length, el.value.length) })
    return () => cancelAnimationFrame(frame)
  }, [composerOpen])
  const mention = body.slice(0, cursor).match(/(?:^|\s)@([\w-]*)$/)
  const suggestions = !dismissed && mention ? roles.filter(r => r.toLowerCase().startsWith(mention[1].toLowerCase())) : []
  const choose = (r: string) => {
    const start = cursor - (mention?.[1].length ?? 0) - 1
    const next = body.slice(0, start) + `@${r} ` + body.slice(cursor)
    setBody(next); setDismissed(true)
    requestAnimationFrame(() => { input.current?.focus(); input.current?.setSelectionRange(start + r.length + 2, start + r.length + 2) })
  }
  const desc = task.text && task.text.trim() !== title(task) ? task.text : ""
  const [editing, setEditing] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [filter, setFilter] = useState<FilterID>("all")
  const [newest, setNewest] = useState(() => { try { return localStorage.getItem(SORT_KEY) !== "oldest" } catch { return true } })
  const toggleSort = () => { setNewest(v => { try { localStorage.setItem(SORT_KEY, v ? "oldest" : "newest") } catch { /* private mode: the choice just is not remembered */ } return !v }) }
  // A notification that offers to reply opens the conversation, not the facts.
  const [tab, setTab] = useState<TabID>(initialReply ? "activity" : "details")
  // Chat can turn unavailable under an open panel (its pane's harness changed): leave it rather than show a dead tab.
  useEffect(() => { if (tab === "chat" && chatOff) setTab("details") }, [tab, chatOff])
  // M opens the composer from anywhere on Activity, as in Jira; typing elsewhere is left alone.
  useEffect(() => {
    if (tab !== "activity") return
    const key = (e: KeyboardEvent) => {
      const el = document.activeElement as HTMLElement | null
      if (e.key !== "m" || e.metaKey || e.ctrlKey || e.altKey || composerOpen || (el && (el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName)))) return
      e.preventDefault(); setComposerOpen(true)
    }
    document.addEventListener("keydown", key)
    return () => document.removeEventListener("keydown", key)
  }, [tab, composerOpen])
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([])
  // Walking the tabs skips a disabled Chat in the direction of travel, so the key
  // moves the cursor the way the reader expects.
  const pickTab = (i: number, dir: 1 | -1 = 1) => {
    let next = ((i % TABS.length) + TABS.length) % TABS.length
    for (let n = 0; n < TABS.length && !tabAllowed(next); n++) next = ((next + dir) % TABS.length + TABS.length) % TABS.length
    if (!tabAllowed(next)) return
    setTab(TABS[next]!.id); tabRefs.current[next]?.focus()
  }
  const op = (body: Record<string, string>) => act(post("/api/tasks", { Dir: dir, ID: task.id, ...body }).then(reload))
  // Dependencies are picked from the board's tickets: the list hides this ticket and the ones already picked; done ones sort last.
  const needs = task.needs ?? []
  const all = project?.tasks ?? []
  const setNeeds = (ids: string[]) => act(post("/api/tasks", { Dir: dir, Op: "needs", ID: task.id, Needs: ids }).then(reload))
  const q = depFilter.trim().toLowerCase()
  const pickable = all.filter(t => t.id !== task.id && !needs.includes(t.id) && `${t.id} ${title(t)}`.toLowerCase().includes(q)).sort((a, b) => Number(a.status === "done") - Number(b.status === "done"))
  const dependents = all.filter(t => t.needs?.includes(task.id))
  // Replying belongs to the conversation: the composer opens prefilled with the @mention.
  const replyTo = (who: string) => { setTab("activity"); setFilter("all"); setBody(`@${who} `); setComposerOpen(true) }
  const reply = () => replyTo(role || roles[0] || "")
  // Start runs the ticket's type (Details > Workflow), or the board's default workflow.
  const startLabel = task.workflow || project?.defaultWorkflow || "default workflow"
  const canSend = !sending && (!!body.trim() || files.length > 0)
  const canStart = (task.status === "todo" || !task.workflow) && task.status !== "done"
  // Roles still working on this ticket after being @mentioned: no reply comment from them since.
  const comments = task.comments ?? []
  const replying = (project?.running?.[task.id] ?? []).filter(r => {
    const asked = comments.findLastIndex(c => mentions(c.body, r))
    return asked >= 0 && asked > comments.findLastIndex(c => c.author === r)
  })
  const rows: Row[] = [
    ...comments.map(c => ({ key: `c${c.id}`, when: c.created, comment: c })),
    ...historyRows(task.workflow ? history : []),
  ].filter(r => filter === "all" || (filter === "comments") === !!r.comment).sort((a, b) => stamp(a.when) - stamp(b.when))
  if (newest) rows.reverse()
  return <>
    <div className="drawer-actions">
      <span className="board-key" title={task.id}>{shortId(task.id)}</span>
      {canStart && <button type="button" className="start-main" title={`Start · ${startLabel}`} onClick={() => op({ Op: "start", Status: "" })}><Play />Start</button>}
      {!!priorityMark(task.priority) && <span className="card-prio" title={`${task.priority} priority`} aria-label={`${task.priority} priority`}>{priorityMark(task.priority)} {task.priority}</span>}
      <button type="button" aria-label="Edit" title="Edit" onClick={() => setEditing(editing === null ? task.text : null)}><Pencil /></button>
      {confirmDelete
        ? <><button type="button" className="danger" onClick={() => op({ Op: "rm" })}><Trash2 />Delete for good</button><button type="button" onClick={() => setConfirmDelete(false)}>Cancel</button></>
        : <button type="button" aria-label="Delete" title="Delete" onClick={() => setConfirmDelete(true)}><Trash2 /></button>}
    </div>
    <DialogTitle className="drawer-title">{title(task)}</DialogTitle>
    <DialogDescription className="sr-only">Task details, status and activity</DialogDescription>
    {task.status === "blocked" && <div className="blocked-reason">{reason || task.note || "Blocked — awaiting a decision"}<button type="button" className="board-send" onClick={reply}><Reply />Reply to unblock</button></div>}
    {editing !== null && <form className="drawer-edit" onSubmit={e => { e.preventDefault(); if (editing.trim()) op({ Op: "edit", Text: editing }).then(() => setEditing(null)) }}>
      <textarea aria-label="Task text" value={editing} onChange={e => setEditing(e.target.value)} rows={8} />
      <div><button className="board-send">Save</button><button type="button" onClick={() => setEditing(null)}>Cancel</button></div>
    </form>}
    <div className="drawer-tabs" role="tablist" aria-label="Ticket panel">
      {TABS.map((t, i) => <button key={t.id} ref={el => { tabRefs.current[i] = el }} type="button" role="tab" id={`task-tab-${t.id}`} aria-selected={tab === t.id} aria-controls={`task-panel-${t.id}`} tabIndex={tab === t.id ? 0 : -1}
        disabled={!tabAllowed(i)} title={t.id === "chat" && chatOff ? chat.info?.reason : undefined}
        onClick={() => setTab(t.id)}
        onKeyDown={e => {
          if (e.key === "ArrowRight" || e.key === "ArrowLeft") { e.preventDefault(); pickTab(i + (e.key === "ArrowRight" ? 1 : -1), e.key === "ArrowRight" ? 1 : -1) }
          if (e.key === "Home" || e.key === "End") { e.preventDefault(); pickTab(e.key === "Home" ? 0 : TABS.length - 1) }
        }}>{t.label}</button>)}
    </div>
    {chatOff && <p role="status" className="-mt-3 flex items-start gap-1.5 rounded-md border border-amber-500/40 bg-amber-500/5 px-2 py-1.5 text-[11.5px] text-amber-700 dark:text-amber-400"><TriangleAlert className="mt-px size-3.5 shrink-0" /><span>Chat unavailable · {chat.info?.reason || "this ticket's chat cannot be read"}</span></p>}
    {tab === "details" && <div role="tabpanel" id="task-panel-details" aria-labelledby="task-tab-details" className="drawer-panel">
      <div className="meta-row">
        <span className="meta-item"><select aria-label="Task status" value={task.status} onChange={e => act(post("/api/tasks", { Dir: dir, Op: "set", ID: task.id, Status: e.target.value }).then(reload))}>{statuses.map(s => <option key={s}>{s}</option>)}</select><i className="status-dot" style={{ background: statusColor(task.status) }} /></span>
        <select aria-label="Task priority" value={task.priority || "normal"} onChange={e => op({ Op: "priority", Priority: e.target.value })}>{["high", "normal", "low"].map(p => <option key={p}>{p}</option>)}</select>
        {role && <span className="meta-item"><Avatar role={role} />{role}</span>}
        {!!task.assignees?.length && <span className="meta-item board-muted" title="Assignees">{task.assignees.join(", ")}</span>}
        <label className="meta-item" title="Start this ticket when its last dependency is done"><input type="checkbox" aria-label="Auto-start" checked={!!task.auto_start} onChange={e => act(post("/api/tasks", { Dir: dir, Op: "auto", ID: task.id, Auto: e.target.checked }).then(reload))} />Auto-start</label>
        <span className="meta-item board-muted" title={task.updated}>updated {rel(task.updated) || "—"}</span>
      </div>
      <div className="workflow-row">
        <span className="board-muted">Workflow</span>
        {task.status === "todo"
          // The type is picked here while the ticket is in todo; Start then runs it.
          ? <select aria-label="Task workflow" className="!normal-case" value={task.workflow || ""} onChange={e => op({ Op: "workflow", Workflow: e.target.value })}>
              {!task.workflow && <option value="">Default ({project?.defaultWorkflow || "none"})</option>}
              {workflows.map(w => <option key={w.name} value={w.name} disabled={!!w.error}>{w.name}{w.global ? " (global)" : ""}{w.error ? " — invalid" : ""}</option>)}
            </select>
          : <span>{task.workflow || "—"}</span>}
        {task.workflow && (() => {
          // Steps of the ticket's recipe: finished ones checked, the current one highlighted.
          const jobs = workflows.find(w => w.name === task.workflow || w.file === task.workflow)?.jobs ?? []
          const at = task.status === "done" ? jobs.length : jobs.findIndex(j => j.status === task.status)
          return <>{jobs.map((j, i) => <span key={j.id} title={j.agent || j.run || ""} className={`rounded px-1.5 py-px text-[11px] ${i === at ? "bg-foreground text-background" : i < at ? "text-emerald-600" : "text-muted-foreground"}`}>
            {i > 0 && <span className="mr-1 text-muted-foreground">›</span>}{i < at && <Check className="mr-0.5 inline size-3" />}{j.status}</span>)}
            <button type="button" onClick={() => setTab("run")} className="inline-flex items-center gap-0.5 text-[11px] text-brand-ink underline-offset-2 hover:underline">View run<ArrowRight className="size-3" /></button></>
        })()}
      </div>
      {/* Usage: one summary line under the meta; open it for one line per job. */}
      {!!usage?.jobs?.length && <section className="usage">
        <button type="button" className="usage-summary" aria-label={`Usage: ${tok(usage.total.Total)} tokens, ${usage.loops} loops, ${dur(usage.elapsed)}`} aria-expanded={usageOpen} onClick={() => setUsageOpen(v => !v)}>
          <span><b>{tok(usage.total.Total)}</b> tokens{usage.cost && <> · <b title="Estimated at API list price, not a bill">≈{usd(usage.cost)}</b></>} · {usage.loops} loops · {dur(usage.elapsed)}</span>
          {usage.partial && <span className="usage-partial" title="Some harnesses report no tokens">partial</span>}
          <ChevronDown className="size-4 shrink-0" style={{ transform: usageOpen ? "rotate(180deg)" : undefined }} />
        </button>
        {(() => {
          const models = byModel(usage.jobs), sum = models.reduce((a, g) => a + g.tokens, 0) || 1
          const color = (j: TicketUsage["jobs"][number]) => models.find(g => g.name === modelName(j))?.color ?? "var(--muted-foreground)"
          return <>
            {!!models.length && <div className="usage-bar" role="img" aria-label={models.map(g => `${g.name} ${Math.round(100 * g.tokens / sum)}%`).join(", ")}>
              {models.map(g => <i key={g.name} title={`${g.name}: ${tok(g.tokens)}`} style={{ width: `${(100 * g.tokens) / sum}%`, background: g.color }} />)}
            </div>}
            <ul className="usage-legend">{models.map(g => <li key={g.name}>
              <i style={{ background: g.color }} />{g.name} <b>{tok(g.tokens)}</b> <span>{Math.round(100 * g.tokens / sum)}%</span>{g.cost && <span>≈{usd(g.cost)}</span>}
            </li>)}</ul>
            {usageOpen && <ul className="usage-rows">{usage.jobs.map((j, i) => <li key={i} title={[j.harness, j.model, j.effort].filter(Boolean).join(" / ")}>
          <span className="usage-role"><i className="usage-dot" style={{ background: color(j) }} />{j.role}</span>
          <span className="usage-model">{(j.model || j.harness || "").split("/").pop()}</span>
          <span className="usage-n">{j.has ? tok(j.count.Total) : "n/a"}</span>
          <span className="usage-n">{j.cost ? `≈${usd(j.cost)}` : ""}</span>
          <span className="usage-n">{dur(j.elapsed)}</span>
        </li>)}</ul>}
          </>
        })()}
      </section>}
      {desc && <section><h3>Description</h3><Md text={desc} /></section>}
      <section>
        <div className="dep-head"><h3>Dependencies{!!needs.length && <span className="board-muted">{needs.length}</span>}</h3>
          <Button type="button" variant="outline" size="xs" aria-expanded={picking} onClick={() => { setPicking(v => !v); setDepFilter("") }}><Plus />Add</Button></div>
        {picking && <div className="dep-picker">
          <Input aria-label="Filter tickets" placeholder="Filter by id or title" value={depFilter} onChange={e => setDepFilter(e.target.value)} />
          <ul className="dep-options" aria-label="Tickets to depend on">{pickable.map(t => <li key={t.id}><button type="button" className={t.status === "done" ? "is-done" : undefined} onClick={() => { setNeeds([...needs, t.id]); setPicking(false) }}><span className="board-key">{shortId(t.id)}</span><span className="dep-title">{title(t)}</span><StatusBadge status={t.status} /></button></li>)}</ul>
          {!pickable.length && <p className="board-muted">No ticket matches.</p>}
        </div>}
        {needs.length
          ? <ul className="dep-list">{needs.map(id => {
              const dep = all.find(t => t.id === id)
              return <li key={id}>
                <button type="button" className="dep-open" onClick={() => onOpen?.(id)}><span className="board-key">{shortId(id)}</span><span className="dep-title">{dep ? title(dep) : "missing"}</span>{dep && <StatusBadge status={dep.status} />}</button>
                <button type="button" className="dep-remove" aria-label={`Remove dependency ${shortId(id)}`} title="Remove" onClick={() => setNeeds(needs.filter(x => x !== id))}><X /></button>
              </li>
            })}</ul>
          : !picking && <p className="board-muted">No dependencies.</p>}
      </section>
      {!!dependents.length && <section><h3>Blocks <span className="board-muted">{dependents.length}</span></h3><ul className="dep-list">{dependents.map(t => <li key={t.id}><button type="button" className="dep-open" onClick={() => onOpen?.(t.id)}><span className="board-key">{shortId(t.id)}</span><span className="dep-title">{title(t)}</span><StatusBadge status={t.status} /></button></li>)}</ul></section>}
      {!!attachments.length && <section><h3>Attachments <span className="board-muted">{attachments.length}</span></h3><AttachmentRows items={attachments} open={setSelected} /></section>}
      {!desc && !attachments.length && <p className="board-muted">No description or attachments yet.</p>}
    </div>}
    {tab === "chat" && <div role="tabpanel" id="task-panel-chat" aria-labelledby="task-tab-chat" className="drawer-panel drawer-chat">
      <TicketChat info={chat.info} error={chat.error} starting={chat.starting} onStart={() => void chat.start()} onReload={() => void chat.reload()} onSave={saveToTicket} project={project} />
    </div>}
    {tab === "activity" && <div role="tabpanel" id="task-panel-activity" aria-labelledby="task-tab-activity" className="drawer-panel">
      {composerOpen
        ? <form className="comment-form" onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); enqueue(Array.from(e.dataTransfer.files)) }} onPaste={e => { const images = Array.from(e.clipboardData.files).filter(f => f.type.startsWith("image/")); if (images.length) { e.preventDefault(); enqueue(images) } }} onSubmit={e => { e.preventDefault(); if (canSend) void submitComment() }}>
          <label htmlFor="task-comment" className="sr-only">Comment</label>
          <textarea id="task-comment" ref={input} value={body} placeholder="Add a comment… type @ to hand off to a role" aria-autocomplete="list" aria-controls={suggestions.length ? "role-suggestions" : undefined} aria-activedescendant={suggestions.length ? `role-option-${index % suggestions.length}` : undefined}
            onSelect={e => setCursor(e.currentTarget.selectionStart)} onChange={e => { setBody(e.target.value); setCursor(e.target.selectionStart); setIndex(0); setDismissed(false) }}
            onKeyDown={e => {
              if ((e.ctrlKey || e.metaKey) && e.key === "Enter") { e.preventDefault(); if (canSend) void submitComment(); return }
              if (suggestions.length) { if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); setDismissed(true) } else if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); setIndex((index + (e.key === "ArrowDown" ? 1 : suggestions.length - 1)) % suggestions.length) } else if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); choose(suggestions[index % suggestions.length]) } return }
              if (e.key === "Escape" && !body.trim() && !files.length) { e.preventDefault(); setComposerOpen(false) }
            }} />
          {!!suggestions.length && <ul id="role-suggestions" role="listbox" aria-label="Mention role">{suggestions.map((r, i) => <li role="option" aria-selected={i === index % suggestions.length} id={`role-option-${i}`} key={r} onMouseDown={e => e.preventDefault()} onClick={() => choose(r)}>@{r}</li>)}</ul>}
          <label className="inline-flex items-center gap-1"><Paperclip className="size-3.5" />Attach files <input type="file" multiple disabled={sending} onChange={e => { enqueue(Array.from(e.target.files ?? [])); e.target.value = "" }} /></label>
          {files.map((f, i) => <button type="button" key={i} onClick={() => setFiles(files.filter((_, j) => i !== j))}>{f.name} ×</button>)}
          {uploadError && <p role="alert">{uploadError}</p>}
          <div className="composer-actions"><button className="board-send" disabled={!canSend}>{sending ? "Sending…" : "Save"}</button><button type="button" onClick={() => setComposerOpen(false)}>Cancel</button></div>
        </form>
        : <button type="button" className="composer-collapsed" onClick={() => setComposerOpen(true)}>Add a comment…</button>}
      <div className="activity-tools">
        <div role="group" aria-label="Filter activity">{FILTERS.map(f => <button key={f.id} type="button" aria-pressed={filter === f.id} className="chip" onClick={() => setFilter(f.id)}>{f.label}</button>)}</div>
        <button type="button" className="sort-toggle" onClick={toggleSort}>{newest ? "Newest first" : "Oldest first"}</button>
      </div>
      {replying.map(r => <p key={r} role="status" className="replying"><Avatar role={r} />{r} replying…</p>)}
      <ul className="board-activity">{rows.map(r => r.comment
        ? <li key={r.key}><Avatar role={r.comment.author} /><div className="comment"><header><b>{r.comment.author}</b><time className="board-muted" title={r.comment.created}>{rel(r.comment.created)}</time>{roles.includes(r.comment.author) && <button type="button" className="reply-link" onClick={() => replyTo(r.comment!.author)}>Reply</button>}</header><Md text={r.comment.body} /><AttachmentRows items={attachments.filter(a => a.commentId === r.comment!.id)} open={setSelected} /></div></li>
        : <li key={r.key} className="history-row">{r.icon}<span>{r.text}</span><time className="board-muted" title={r.when}>{rel(r.when)}</time></li>)}</ul>
      {!rows.length && <p className="board-muted">{filter === "history" ? "No history yet." : "No activity yet."}</p>}
    </div>}
    {tab === "run" && <div role="tabpanel" id="task-panel-run" aria-labelledby="task-tab-run" className="drawer-panel">
      {task.workflow
        ? <RunView dir={dir} id={task.id} />
        : <p className="board-muted">This ticket has no workflow, so there is no run to show.</p>}
    </div>}
    {selected && <AttachmentViewer key={selected.ref} a={selected} url={url} close={() => setSelected(null)} />}
  </>
}
