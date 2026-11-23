import { useRef, useState } from "react"
import { ListTodo, Plus } from "lucide-react"
import { act, post } from "@/api"
import { TaskDrawer } from "@/components/Board"
import { Avatar, shortId, statusColor, title } from "@/components/BoardDrawer"
import { Panel } from "@/components/Panel"
import { Input } from "@/components/ui/input"
import type { Project, Task } from "@/types"
import "../board.css"

// TasksPanel is the compact list of the board: one row per ticket, ordered by board column, newest first.
export function TasksPanel({ proj, dir, reload, className }: { proj?: Project; dir: string; reload: () => void; className?: string }) {
  const tasks = proj?.tasks ?? []
  const statuses = proj?.statuses ?? []
  const role = (t: Task) => proj?.workers?.[t.id]?.role || statuses.find(s => s.id === t.status)?.role || t.assignees?.[0] || ""
  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState("")
  const [openId, setOpenId] = useState<string>()
  const [showDone, setShowDone] = useState(false)
  const origin = useRef<HTMLElement | null>(null)
  const order = (t: Task) => { const i = statuses.findIndex(s => s.id === t.status); return i < 0 ? statuses.length : i }
  const sorted = [...tasks].sort((a, b) => order(a) - order(b) || (b.updated || "").localeCompare(a.updated || ""))
  const done = sorted.filter(t => t.status === "done")
  const shown = showDone ? sorted : sorted.filter(t => t.status !== "done")
  return <Panel className={className} icon={<ListTodo className="text-amber-600" />} title={<a href="#/" className="hover:underline">Tasks</a>} right={<><a href="#/board" className="rounded px-1.5 py-1 text-xs hover:bg-accent">Board</a><span>{tasks.length}</span><button aria-label="Add task" className="flex size-6 items-center justify-center rounded-md hover:bg-accent" onClick={() => setAdding(!adding)}><Plus className="size-3.5" /></button></>} bodyClass="py-1">
    {adding && <form className="px-2 pb-1" onSubmit={e => { e.preventDefault(); if (draft.trim()) act(post("/api/tasks", { Dir: dir, Op: "add", Text: draft.trim() }).then(() => { setDraft(""); setAdding(false); reload() })) }}><Input autoFocus value={draft} onChange={e => setDraft(e.target.value)} placeholder="New task…" className="h-8 text-[12.5px]" /></form>}
    {!tasks.length && <p className="px-2.5 py-2 text-xs text-muted-foreground">No tasks yet.</p>}
    {shown.map(t => <button key={t.id} onClick={e => { origin.current = e.currentTarget; setOpenId(t.id) }} title={`${t.id} · ${t.status}`}
      className={`flex h-8 w-full items-center gap-2 px-2.5 text-left text-[12.5px] hover:bg-accent ${t.status === "done" ? "text-muted-foreground" : ""}`} aria-label={`${shortId(t.id)} ${t.status}: ${title(t)}`}>
      <i className="status-dot" style={{ background: statusColor(t.status) }} />
      <span className="board-key w-10 shrink-0">{shortId(t.id)}</span>
      <span className="min-w-0 flex-1 truncate">{title(t)}</span>
      {t.status === "blocked" && <span className="shrink-0 text-[11px] text-red-600">blocked</span>}
      {proj?.workers?.[t.id]?.live && <span className="board-live" title="Agent working">●</span>}
      <Avatar role={role(t)} />
    </button>)}
    {!!done.length && <button className="flex h-8 w-full items-center px-2.5 text-[11.5px] text-muted-foreground hover:text-foreground" onClick={() => setShowDone(!showDone)}>{showDone ? "Hide done" : `Show ${done.length} done`}</button>}
    <TaskDrawer proj={proj} dir={dir} reload={reload} id={openId} onClose={() => setOpenId(undefined)} origin={origin} />
  </Panel>
}
