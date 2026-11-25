import { useRef, useState } from "react"
import { ListTodo, Plus } from "lucide-react"
import { act, post } from "@/api"
import { TaskDrawer } from "@/components/Board"
import { Avatar, shortId, statusColor, title } from "@/components/BoardDrawer"
import { Panel } from "@/components/Panel"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { baseName } from "@/lib/status"
import type { Project, Task } from "@/types"
import "../board.css"

// TasksPanel is the compact list of the board: one row per ticket, ordered by board column, newest first.
export function TasksPanel({ proj, dir, reload, className }: { proj?: Project; dir: string; reload: () => void; className?: string }) {
  const tasks = proj?.tasks ?? []
  const statuses = proj?.statuses ?? []
  const role = (t: Task) => proj?.workers?.[t.id]?.role || statuses.find(s => s.id === t.status)?.role || t.assignees?.[0] || ""
  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState("")
  const [kind, setKind] = useState("") // the new ticket's type: a workflow name, or none
  const [openId, setOpenId] = useState<string>()
  const [showDone, setShowDone] = useState(false)
  const origin = useRef<HTMLElement | null>(null)
  const order = (t: Task) => { const i = statuses.findIndex(s => s.id === t.status); return i < 0 ? statuses.length : i }
  const sorted = [...tasks].sort((a, b) => order(a) - order(b) || (b.updated || "").localeCompare(a.updated || ""))
  const done = sorted.filter(t => t.status === "done")
  // Fewer than 10 open tickets: the most recently updated done ones fill the list up to 10.
  const open = sorted.filter(t => t.status !== "done")
  const shown = showDone ? sorted : [...open, ...done.slice(0, Math.max(0, 10 - open.length))]
  const hidden = sorted.length - shown.length
  return <Panel className={className} icon={<ListTodo className="text-amber-600" />} title={<a href="#/" className="hover:underline">Tasks</a>} right={<><span>{tasks.length}</span><button aria-label="Add task" className="flex size-6 items-center justify-center rounded-md hover:bg-accent" onClick={() => { setKind(proj?.defaultWorkflow ?? ""); setAdding(!adding) }}><Plus className="size-3.5" /></button></>} bodyClass="py-1">
    {adding && <form className="flex gap-1.5 px-2 pb-1" onSubmit={e => { e.preventDefault(); if (draft.trim()) act(post("/api/tasks", { Dir: dir, Op: "add", Text: draft.trim(), Workflow: kind }).then(() => { setDraft(""); setAdding(false); reload() })) }}>
      <Input autoFocus value={draft} onChange={e => setDraft(e.target.value)} placeholder="New task…" className="h-8 min-w-0 flex-1 text-[12.5px]" />
      {/* Radix Select cannot take "" as an item value: "__none" stands for no workflow and never leaves this component. */}
      <Select value={kind || "__none"} onValueChange={v => setKind(v === "__none" ? "" : v)}>
        <SelectTrigger size="sm" aria-label="Task type" className="h-8 max-w-[40%] text-[12.5px]"><SelectValue /></SelectTrigger>
        <SelectContent>
          <SelectItem value="__none">no workflow</SelectItem>
          {(proj?.workflows ?? []).map(w => <SelectItem key={w.name} value={w.name} disabled={!!w.error}>{w.name}{w.global ? " (global)" : ""}</SelectItem>)}
        </SelectContent>
      </Select>
    </form>}
    {!tasks.length && <p className="px-2.5 py-2 text-xs text-muted-foreground">No tasks in {baseName(dir)}.</p>}
    {shown.map(t => <button key={t.id} onClick={e => { origin.current = e.currentTarget; setOpenId(t.id) }} title={`${t.id} · ${t.status}`}
      className={`flex h-8 w-full items-center gap-2 px-2.5 text-left text-[12.5px] hover:bg-accent ${t.status === "done" ? "text-muted-foreground" : ""}`} aria-label={`${shortId(t.id)} ${t.status}: ${title(t)}`}>
      <i className="status-dot" style={{ background: statusColor(t.status) }} />
      <span className="board-key w-10 shrink-0">{shortId(t.id)}</span>
      <span className="min-w-0 flex-1 truncate">{title(t)}</span>
      {t.status === "blocked" && <span className="shrink-0 text-[11px] text-red-600">blocked</span>}
      {proj?.workers?.[t.id]?.live && <span className="board-live" title="Agent working">●</span>}
      <Avatar role={role(t)} />
    </button>)}
    {(showDone ? !!done.length : hidden > 0) && <button className="flex h-8 w-full items-center px-2.5 text-[11.5px] text-muted-foreground hover:text-foreground" onClick={() => setShowDone(!showDone)}>{showDone ? "Hide done" : `Show ${hidden} more done`}</button>}
    <TaskDrawer proj={proj} dir={dir} reload={reload} id={openId} onClose={() => setOpenId(undefined)} onOpen={setOpenId} origin={origin} />
  </Panel>
}
