import { useState } from "react"
import { Workflow as WorkflowIcon } from "lucide-react"
import { api } from "@/api"
import { Panel } from "@/components/Panel"
import { WorkflowsPage, type Flow } from "@/components/WorkflowsPage"
import { usePoll } from "@/hooks/usePoll"
import { enc } from "@/hooks/useRoute"
import { useSize } from "@/hooks/useSize"

// The editor needs about 900px of pane width; a narrower pane lists the workflows and opens them maximized.
const EDITOR_MIN_WIDTH = 900

// WorkflowsPane is the Workflows tab of the Tasks column. A dirty editor stays up, so a resize never drops a draft.
export function WorkflowsPane({ dir }: { dir: string }) {
  const [ref, { w }] = useSize()
  const [dirty, setDirty] = useState(false)
  return <div ref={ref} className="flex min-h-0 flex-1 flex-col">
    {w >= EDITOR_MIN_WIDTH || dirty ? <WorkflowsPage dir={dir} onDirty={setDirty} /> : <WorkflowList dir={dir} />}
  </div>
}

function WorkflowList({ dir }: { dir: string }) {
  const poll = usePoll(() => api<Flow[]>("/api/workflows?dir=" + enc(dir)), 2000, dir)
  const flows = poll.data ?? []
  return <Panel className="min-h-0 flex-1" icon={<WorkflowIcon className="text-teal-600" />} title="Workflows" bodyClass="py-1">
    {poll.error ? <p role="alert" className="px-2.5 py-2 text-xs text-destructive">Couldn't load workflows. {poll.error.message}</p>
      : !poll.data ? <p className="px-2.5 py-2 text-xs text-muted-foreground">Loading…</p>
      : !flows.length ? <p className="px-2.5 py-2 text-xs text-muted-foreground">No workflows yet.</p>
      : flows.map(f => <a key={f.key + f.global} href={`#/workflows/${enc(f.key)}`} className="flex flex-col gap-0.5 px-2.5 py-1.5 hover:bg-accent">
        <span className="flex items-center gap-1.5 text-[12.5px] font-medium">{f.name}{f.global && <span className="text-[10.5px] font-normal text-muted-foreground">global</span>}{f.running > 0 && <span className="ml-auto text-[11px] text-emerald-600">● {f.running}</span>}</span>
        <span className="truncate text-[11.5px] text-muted-foreground">{f.description || `${f.jobs?.length ?? 0} steps`}</span>
      </a>)}
  </Panel>
}
