import { useRef, useState } from "react"
import { KanbanSquare, OctagonAlert, Search } from "lucide-react"
import { Avatar, BoardDrawer, shortId, statusColor, title } from "./BoardDrawer"
import { Dialog, DialogContent } from "@/components/ui/dialog"
import { act, post } from "@/api"
import type { Project, Task } from "@/types"
import { Workflows } from "./Workflows"
import "./board.css"

// TaskDrawer is the Jira-like right panel shared by the board and the tasks list.
export function TaskDrawer({ proj, dir, reload, id, onClose, origin }: { proj?: Project; dir: string; reload: () => void; id?: string; onClose: () => void; origin: React.RefObject<HTMLElement | null> }) {
  const statuses = proj?.statuses ?? []
  const selected = proj?.tasks?.find(t => t.id === id)
  const role = (t: Task) => proj?.workers?.[t.id]?.role || statuses.find(s => s.id === t.status)?.role || t.assignees?.[0] || ""
  const roles = Array.from(new Set([...(proj?.roles ?? []), ...(proj?.tasks ?? []).map(role)])).filter(Boolean)
  return <Dialog open={!!selected} onOpenChange={open => { if (!open) onClose() }}>
    <DialogContent className="board-drawer translate-x-0 translate-y-0 sm:max-w-none" onOpenAutoFocus={e => { e.preventDefault(); (e.currentTarget as HTMLElement).focus() }} onCloseAutoFocus={e => { e.preventDefault(); if (origin.current?.isConnected) origin.current.focus() }}>
      {selected && <BoardDrawer key={selected.id} workflows={proj?.workflows ?? []} task={selected} statuses={statuses.map(s => s.id)} roles={roles} role={role(selected)} reason={proj?.blockedReasons?.[selected.id]} dir={dir} reload={reload} />}
    </DialogContent>
  </Dialog>
}

export function Board({ dir, proj, reload }: { dir: string; proj?: Project; reload: () => void }) {
  const [query, setQuery] = useState("")
  const [role, setRole] = useState("")
  const [blocked, setBlocked] = useState(false)
  const [selectedID, setSelectedID] = useState<string>()
  const [over, setOver] = useState("")
  const origin = useRef<HTMLElement | null>(null)
  const statuses = proj?.statuses ?? []
  const taskRole = (t: Task) => proj?.workers?.[t.id]?.role || statuses.find(s => s.id === t.status)?.role || t.assignees?.[0] || ""
  const roles = Array.from(new Set([...(proj?.roles ?? []), ...(proj?.tasks ?? []).map(taskRole)])).filter(Boolean)
  const all = proj?.tasks ?? []
  const tasks = all.filter(t => (!blocked || t.status === "blocked") && (!role || taskRole(t) === role || t.assignees?.includes(role)) && (!query || `${t.id} ${t.title} ${t.text}`.toLowerCase().includes(query.toLowerCase())))
  const move = (id: string, status: string) => act(post("/api/tasks", { Dir: dir, Op: "set", ID: id, Status: status }).then(reload))
  return <section className="task-board">
    <header className="board-tools">
      <h2><span className="board-icon"><KanbanSquare /></span>Board</h2>
      <label className="board-search"><Search /><input aria-label="Search tasks" placeholder="Search" value={query} onChange={e => setQuery(e.target.value)} /></label>
      <select aria-label="Filter role" value={role} onChange={e => setRole(e.target.value)}><option value="">All roles</option>{roles.map(r => <option key={r}>{r}</option>)}</select>
      <button aria-pressed={blocked} onClick={() => setBlocked(!blocked)}><OctagonAlert />Blocked</button>
      <Workflows workflows={proj?.workflows ?? []} />
      <span className="board-muted ml-auto">{tasks.length === all.length ? `${all.length} tasks` : `${tasks.length} of ${all.length}`}</span>
    </header>
    {!statuses.length && <p className="board-empty">{proj ? "No board columns configured." : "Select a project to view tasks."}</p>}
    <div className="board-columns">{statuses.map(s => {
      const column = tasks.filter(t => t.status === s.id).sort((a, b) => (b.updated || "").localeCompare(a.updated || ""))
      const live = column.some(t => proj?.workers?.[t.id]?.live)
      return <section key={s.id} aria-label={`${s.id} column`} className={`board-column ${column.length ? "" : "is-empty"} ${over === s.id ? "drop-over" : ""}`}
        onDragOver={e => { e.preventDefault(); setOver(s.id) }} onDragLeave={e => { if (!e.currentTarget.contains(e.relatedTarget as Node)) setOver("") }} onDrop={e => { e.preventDefault(); setOver(""); const id = e.dataTransfer.getData("text/task"); if (id) move(id, s.id) }}>
        <header className="column-heading">
          <i className="status-dot" style={{ background: statusColor(s.id) }} />
          <h3>{s.id}</h3><span className="column-count">{column.length}</span>
          {live && <span className="board-live" title="Agent working">●</span>}
          <span className="column-owner">{s.role ? <Avatar role={s.role} /> : s.run ? <span className="board-key" title={s.run}>CI</span> : null}</span>
        </header>
        <div className="board-cards">{column.map(t => {
          const working = proj?.workers?.[t.id]?.live
          return <button type="button" key={t.id} draggable onDragStart={e => { e.dataTransfer.setData("text/task", t.id); e.dataTransfer.effectAllowed = "move" }} onDragEnd={() => setOver("")}
            onClick={e => { origin.current = e.currentTarget; setSelectedID(t.id) }} aria-label={`${shortId(t.id)}: ${title(t)}`} className={`board-card ${t.status === "blocked" ? "is-blocked" : ""}`}>
            <span className="card-title">{title(t)}</span>
            {t.status === "blocked" && <span className="blocked-reason">{proj?.blockedReasons?.[t.id] || t.note || "Awaiting a decision"}</span>}
            <span className="card-meta">
              <span className="board-key" title={t.id}>{shortId(t.id)}</span>
              {t.workflow && <span className="card-label">{t.workflow}</span>}
              {!!t.comments?.length && <span aria-label={`${t.comments.length} comments`}>💬 {t.comments.length}</span>}
              {working && <span className="board-live">● working</span>}
              <Avatar role={taskRole(t)} />
            </span>
          </button>
        })}{!column.length && <p className="board-empty">{query || role || blocked ? "No match" : "Empty"}</p>}</div>
      </section>
    })}</div>
    <TaskDrawer proj={proj} dir={dir} reload={reload} id={selectedID} onClose={() => setSelectedID(undefined)} origin={origin} />
  </section>
}
