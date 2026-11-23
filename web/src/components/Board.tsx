import { useEffect, useRef, useState } from "react"
import { KanbanSquare, MessageSquare, Search, Settings } from "lucide-react"
import { Avatar, BoardDrawer, needState, priorityMark, priorityRank, shortId, statusColor, title } from "./BoardDrawer"
import { Dialog, DialogContent } from "@/components/ui/dialog"
import { act, post } from "@/api"
import type { Project, Task } from "@/types"
import "./board.css"

function useMedia(q: string) {
  const [on, setOn] = useState(() => matchMedia(q).matches)
  useEffect(() => { const m = matchMedia(q); const f = () => setOn(m.matches); m.addEventListener("change", f); return () => m.removeEventListener("change", f) }, [q])
  return on
}

// TaskDrawer is the Jira-like right panel shared by the board and the tasks list.
export function TaskDrawer({ proj, dir, reload, id, onClose, onOpen, origin, initialReply = '' }: { initialReply?: string; onOpen?: (id: string) => void; proj?: Project; dir: string; reload: () => void; id?: string; onClose: () => void; origin: React.RefObject<HTMLElement | null> }) {
  const statuses = proj?.statuses ?? []
  const selected = proj?.tasks?.find(t => t.id === id)
  const role = (t: Task) => proj?.workers?.[t.id]?.role || statuses.find(s => s.id === t.status)?.role || t.assignees?.[0] || ""
  const roles = Array.from(new Set([...(proj?.roles ?? []), ...(proj?.tasks ?? []).map(role)])).filter(Boolean)
  // Desktop: docked beside the board, non-modal (no overlay, the board stays clickable). Phones keep the full-screen modal.
  const docked = useMedia("(min-width: 768px)")
  return <Dialog modal={!docked} open={!!selected} onOpenChange={open => { if (!open) onClose() }}>
    <DialogContent className="board-drawer translate-x-0 translate-y-0 sm:max-w-none" onInteractOutside={e => { if (docked) e.preventDefault() }} onOpenAutoFocus={e => { e.preventDefault(); (e.currentTarget as HTMLElement).focus() }} onCloseAutoFocus={e => { e.preventDefault(); if (origin.current?.isConnected) origin.current.focus() }}>
      {selected && <BoardDrawer key={selected.id + initialReply} initialReply={initialReply} project={proj} workflows={proj?.workflows ?? []} task={selected} statuses={statuses.map(s => s.id)} roles={roles} role={role(selected)} reason={proj?.blockedReasons?.[selected.id]} dir={dir} reload={reload} onOpen={onOpen} />}
    </DialogContent>
  </Dialog>
}

const LANES = ["todo", "in progress", "blocked", "done"]
const laneOf = (status: string) => (status === "todo" || status === "blocked" || status === "done" ? status : "in progress")

export function Board({ dir, proj, reload }: { dir: string; proj?: Project; reload: () => void }) {
  const [query, setQuery] = useState("")
  const [role, setRole] = useState("")
  const [selectedID, setSelectedID] = useState<string>()
  const [over, setOver] = useState("")
  const origin = useRef<HTMLElement | null>(null)
  const statuses = proj?.statuses ?? []
  const taskRole = (t: Task) => proj?.workers?.[t.id]?.role || statuses.find(s => s.id === t.status)?.role || t.assignees?.[0] || ""
  const roles = Array.from(new Set([...(proj?.roles ?? []), ...(proj?.tasks ?? []).map(taskRole)])).filter(Boolean)
  const all = proj?.tasks ?? []
  const tasks = all.filter(t => (!role || taskRole(t) === role || t.assignees?.includes(role)) && (!query || `${t.id} ${t.title} ${t.text}`.toLowerCase().includes(query.toLowerCase())))
  const move = (id: string, status: string) => act(post("/api/tasks", { Dir: dir, Op: "set", ID: id, Status: status }).then(reload))
  return <section className={`task-board ${selectedID ? "has-drawer" : ""}`}>
    <header className="board-tools">
      <h2><span className="board-icon"><KanbanSquare /></span>Board</h2>
      {proj?.defaultWorkflow && <a href="#/board/config" className="board-muted" title="Default workflow">default: {proj.defaultWorkflow}</a>}
      <label className="board-search"><Search /><input aria-label="Search tasks" placeholder="Search" value={query} onChange={e => setQuery(e.target.value)} /></label>
      <select aria-label="Filter role" value={role} onChange={e => setRole(e.target.value)}><option value="">All roles</option>{roles.map(r => <option key={r}>{r}</option>)}</select>
      <a href="#/board/config" className="board-settings" aria-label="Board settings" title="Board settings"><Settings /></a>
      <span className="board-muted ml-auto">{tasks.length === all.length ? `${all.length} tasks` : `${tasks.length} of ${all.length}`}</span>
    </header>
    {!statuses.length && <p className="board-empty">{proj ? "No board columns configured." : "Select a project to view tasks."}</p>}
    <div className="board-columns">{LANES.map(lane => {
      const column = tasks.filter(t => laneOf(t.status) === lane).sort((a, b) => priorityRank(a.priority) - priorityRank(b.priority) || (b.updated || "").localeCompare(a.updated || ""))
      const live = column.some(t => proj?.workers?.[t.id]?.live)
      // "in progress" spans every working status, so a drop there has no single target status.
      const drop = lane !== "in progress"
      return <section key={lane} aria-label={`${lane} column`} className={`board-column ${column.length ? "" : "is-empty"} ${over === lane ? "drop-over" : ""}`}
        onDragOver={e => { if (drop) { e.preventDefault(); setOver(lane) } }} onDragLeave={e => { if (!e.currentTarget.contains(e.relatedTarget as Node)) setOver("") }} onDrop={e => { e.preventDefault(); setOver(""); const id = e.dataTransfer.getData("text/task"); if (id && drop) move(id, lane) }}>
        <header className="column-heading">
          <i className="status-dot" style={{ background: statusColor(lane === "in progress" ? "build" : lane) }} />
          <h3>{lane}</h3><span className="column-count">{column.length}</span>
          {live && <span className="board-live" title="Agent working">●</span>}
        </header>
        <div className="board-cards">{column.map(t => {
          const working = proj?.workers?.[t.id]?.live
          const state = needState(t, all)
          return <button type="button" key={t.id} draggable onDragStart={e => { e.dataTransfer.setData("text/task", t.id); e.dataTransfer.effectAllowed = "move" }} onDragEnd={() => setOver("")}
            onClick={e => { origin.current = e.currentTarget; setSelectedID(t.id) }} aria-label={`${shortId(t.id)}: ${title(t)}`} className={`board-card ${t.status === "blocked" ? "is-blocked" : ""}`}>
            <span className="card-title">{title(t)}</span>
            {t.status === "blocked" && <span className="blocked-reason">{proj?.blockedReasons?.[t.id] || t.note || "Awaiting a decision"}</span>}
            <span className="card-meta">
              <span className="board-key" title={t.id}>{shortId(t.id)}</span>
              {!!priorityMark(t.priority) && <span className="card-prio" title={`${t.priority} priority`} aria-label={`${t.priority} priority`}>{priorityMark(t.priority)}</span>}
              {!!state && <span className={`card-deps ${state === "ready" ? "is-ready" : ""}`}>{state}</span>}
              {lane === "in progress" && <span className="card-label" style={{ color: statusColor(t.status) }}>● {t.status}</span>}
              {t.workflow && <span className="card-label">{t.workflow}</span>}
              {!!t.comments?.length && <span className="inline-flex items-center gap-0.5" aria-label={`${t.comments.length} comments`}><MessageSquare className="size-3" />{t.comments.length}</span>}
              {working && <span className="board-live">● working</span>}
              <Avatar role={taskRole(t)} />
            </span>
          </button>
        })}{!column.length && <p className="board-empty">{query || role ? "No match" : "Empty"}</p>}</div>
      </section>
    })}</div>
    <TaskDrawer proj={proj} dir={dir} reload={reload} id={selectedID} onClose={() => setSelectedID(undefined)} onOpen={setSelectedID} origin={origin} />
  </section>
}
