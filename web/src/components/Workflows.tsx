import { useState } from "react"
import { Workflow as WorkflowIcon } from "lucide-react"
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog"
import { Avatar, statusColor } from "./BoardDrawer"
import type { Workflow } from "@/types"

// Workflows lists the project's and global recipes with their job chain. Editing stays in the TUI ($EDITOR).
export function Workflows({ workflows }: { workflows: Workflow[] }) {
  const [open, setOpen] = useState(false)
  return <>
    <button type="button" onClick={() => setOpen(true)}><WorkflowIcon className="!text-sky-600" />Workflows <span className="board-muted">{workflows.length}</span></button>
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="board-drawer translate-x-0 translate-y-0 sm:max-w-none" onOpenAutoFocus={e => { e.preventDefault(); (e.currentTarget as HTMLElement).focus() }}>
        <DialogTitle className="drawer-title !mt-0">Workflows</DialogTitle>
        <DialogDescription className="board-muted">Start one from a ticket (Start with…). Create or edit them in the TUI: board, w, then a / e.</DialogDescription>
        {!workflows.length && <p className="board-muted">No workflows yet. Add one with <code>tagent workflow new NAME</code>.</p>}
        <ul className="workflow-list">{workflows.map(w => <li key={w.file}>
          <header><b>{w.name}</b><span className="card-label">{w.global ? "global" : "project"}</span></header>
          {w.description && <p>{w.description}</p>}
          {w.error ? <p className="blocked-reason">{w.error}</p> : <ol className="workflow-jobs">{w.jobs.map((j, i) => <li key={j.id}>
            {i > 0 && <span className="board-muted">→</span>}
            <span className="workflow-job"><i className="status-dot" style={{ background: statusColor(j.status) }} />{j.id}{j.agent ? <Avatar role={j.agent} /> : j.run ? <span className="board-key">CI</span> : null}</span>
          </li>)}</ol>}
          <span className="board-key" title={w.file}>{w.file.split("/").slice(-3).join("/")}</span>
        </li>)}</ul>
      </DialogContent>
    </Dialog>
  </>
}
