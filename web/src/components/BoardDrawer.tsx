import { Fragment, useEffect, useRef, useState, type ReactNode } from "react"
import { DialogTitle, DialogDescription } from "@/components/ui/dialog"
import { act, post } from "@/api"
import { Pencil, Play, Reply, Trash2 } from "lucide-react"
import type { Task, Workflow } from "@/types"

// Status colours are reserved and always sit next to a text label.
const COLORS: Record<string, string> = { todo: "#a1a1aa", plan: "#3b82f6", planning: "#3b82f6", researching: "#3b82f6", reproducing: "#3b82f6", build: "#6366f1", building: "#6366f1", refactoring: "#6366f1", fixing: "#6366f1", writing: "#6366f1", test: "#0ea5e9", testing: "#0ea5e9", review: "#f59e0b", reviewing: "#f59e0b", blocked: "#ef4444", done: "#10b981" }
export const statusColor = (s: string) => COLORS[s] ?? "#a1a1aa"
export function StatusBadge({ status }: { status: string }) {
  return <span className="status-badge"><i style={{ background: statusColor(status) }} />{status}</span>
}
// shortId keeps ticket keys readable: numeric ids as #21, generated ids by their last 4 characters.
export const shortId = (id: string) => "#" + (/^\d+$/.test(id) ? id : id.slice(-4))
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

export function BoardDrawer({ task, statuses, roles, role, reason, dir, reload, workflows, initialReply = '' }: { initialReply?: string; task: Task; statuses: string[]; roles: string[]; role: string; reason?: string; dir: string; reload: () => void; workflows: Workflow[] }) {
  const [body, setBody] = useState(initialReply)
  const [sending, setSending] = useState(false)
  const [cursor, setCursor] = useState(0)
  const [index, setIndex] = useState(0)
  const [dismissed, setDismissed] = useState(false)
  const input = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    if (!initialReply) return
    const frame = requestAnimationFrame(() => { input.current?.focus({ preventScroll: true }); input.current?.setSelectionRange(initialReply.length, initialReply.length) })
    return () => cancelAnimationFrame(frame)
  }, [initialReply])
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
  const [flow, setFlow] = useState(workflows.find(w => !w.error)?.name ?? "board")
  const op = (body: Record<string, string>) => act(post("/api/tasks", { Dir: dir, ID: task.id, ...body }).then(reload))
  const reply = () => { const who = role || roles[0] || ""; setBody(`@${who} `); requestAnimationFrame(() => { input.current?.focus(); input.current?.setSelectionRange(who.length + 2, who.length + 2) }) }
  return <>
    <div className="drawer-actions">
      <span className="board-key" title={task.id}>{shortId(task.id)}</span>
      <button type="button" onClick={() => setEditing(editing === null ? task.text : null)}><Pencil />Edit</button>
      {confirmDelete
        ? <><button type="button" className="danger" onClick={() => op({ Op: "rm" })}><Trash2 />Delete for good</button><button type="button" onClick={() => setConfirmDelete(false)}>Cancel</button></>
        : <button type="button" onClick={() => setConfirmDelete(true)}><Trash2 />Delete</button>}
    </div>
    <DialogTitle className="drawer-title">{title(task)}</DialogTitle>
    <DialogDescription className="sr-only">Task details, status and activity</DialogDescription>
    {task.status === "blocked" && <div className="blocked-reason">{reason || task.note || "Blocked — awaiting a decision"}<button type="button" className="board-send" onClick={reply}><Reply />Reply to unblock</button></div>}
    {(task.status === "todo" || !task.workflow) && task.status !== "done" && <div className="drawer-start">
      <Play />
      <select aria-label="Start with" value={flow} onChange={e => setFlow(e.target.value)}>
        <option value="board">Next board column</option>
        {workflows.map(w => <option key={w.name} value={w.name} disabled={!!w.error}>{w.name}{w.global ? " (global)" : ""}{w.error ? " — invalid" : ""}</option>)}
      </select>
      <button type="button" className="board-send" onClick={() => op({ Op: "start", Status: flow })}>Start</button>
    </div>}
    {editing !== null && <form className="drawer-edit" onSubmit={e => { e.preventDefault(); if (editing.trim()) op({ Op: "edit", Text: editing }).then(() => setEditing(null)) }}>
      <textarea aria-label="Task text" value={editing} onChange={e => setEditing(e.target.value)} rows={8} />
      <div><button className="board-send">Save</button><button type="button" onClick={() => setEditing(null)}>Cancel</button></div>
    </form>}
    <dl className="board-fields">
      <dt>Status</dt><dd><i className="status-dot" style={{ background: statusColor(task.status) }} /><select aria-label="Task status" value={task.status} onChange={e => act(post("/api/tasks", { Dir: dir, Op: "set", ID: task.id, Status: e.target.value }).then(reload))}>{statuses.map(s => <option key={s}>{s}</option>)}</select></dd>
      {role && <><dt>Role</dt><dd><Avatar role={role} />{role}</dd></>}
      {task.workflow && <><dt>Workflow</dt><dd>{task.workflow}</dd></>}
      {!!task.assignees?.length && <><dt>Assignees</dt><dd>{task.assignees.join(", ")}</dd></>}
      <dt>Updated</dt><dd title={task.updated}>{rel(task.updated) || "—"}</dd>
    </dl>
    {desc && <section><h3>Description</h3><Md text={desc} /></section>}
    <section>
      <h3>Activity <span className="board-muted">{task.comments?.length ?? 0}</span></h3>
      <ul className="board-activity">{task.comments?.map((c, i) => <li key={c.id ?? i}><Avatar role={c.author} /><div className="comment"><header><b>{c.author}</b><time className="board-muted" title={c.created}>{rel(c.created)}</time></header><Md text={c.body} /></div></li>)}</ul>
      {!task.comments?.length && <p className="board-muted">No activity yet.</p>}
    </section>
    <form className="comment-form" onSubmit={e => { e.preventDefault(); if (!body.trim() || sending) return; setSending(true); act(post("/api/tasks", { Dir: dir, Op: "comment", ID: task.id, Text: body }).then(() => { setBody(""); reload() }).finally(() => setSending(false))) }}>
      <label htmlFor="task-comment" className="sr-only">Comment</label>
      <textarea id="task-comment" ref={input} value={body} placeholder="Comment… type @role to hand off" aria-autocomplete="list" aria-controls={suggestions.length ? "role-suggestions" : undefined} aria-activedescendant={suggestions.length ? `role-option-${index % suggestions.length}` : undefined}
        onSelect={e => setCursor(e.currentTarget.selectionStart)} onChange={e => { setBody(e.target.value); setCursor(e.target.selectionStart); setIndex(0); setDismissed(false) }}
        onKeyDown={e => { if (!suggestions.length) return; if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); setDismissed(true) } else if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); setIndex((index + (e.key === "ArrowDown" ? 1 : suggestions.length - 1)) % suggestions.length) } else if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); choose(suggestions[index % suggestions.length]) } }} />
      {!!suggestions.length && <ul id="role-suggestions" role="listbox" aria-label="Mention role">{suggestions.map((r, i) => <li role="option" aria-selected={i === index % suggestions.length} id={`role-option-${i}`} key={r} onMouseDown={e => e.preventDefault()} onClick={() => choose(r)}>@{r}</li>)}</ul>}
      <button className="board-send" disabled={sending || !body.trim()}>{sending ? "Sending…" : "Comment"}</button>
    </form>
  </>
}
