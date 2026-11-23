import { act, post } from "@/api"
import { Panel } from "@/components/Panel"
import { enc, nav } from "@/hooks/useRoute"
import type { Project } from "@/types"

export function DefsPanel({ proj, dir, className }: { proj?: Project; dir: string; className?: string }) {
  const roles = proj?.roles ?? []
  const start = (role: string) => act(post<{ pane: string }>("/api/agents", {
    Agent: "", Model: "", Cwd: dir, Role: role, Name: "", Prompt: "", Worktree: false,
  }).then((r) => nav("/agent/" + enc(r.pane))))
  return <Panel className={className} icon="◇" title="Agents" bodyClass="py-1">
    {!roles.length && <p className="px-2.5 py-2 text-xs text-muted-foreground">No roles in this project.</p>}
    {roles.map((role) => <div key={role} className="flex items-center gap-2 px-2.5 py-1.5 text-xs hover:bg-accent">
      <span className="text-muted-foreground">⌂</span>
      <span className="min-w-0 flex-1 truncate">{role}</span>
      <button type="button" className="rounded px-1 hover:bg-accent" onClick={() => void start(role)}>start</button>
    </div>)}
  </Panel>
}
