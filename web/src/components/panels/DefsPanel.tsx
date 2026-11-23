import { Play, Users } from "lucide-react"
import { act, post } from "@/api"
import { Avatar } from "@/components/BoardDrawer"
import { Panel } from "@/components/Panel"
import { enc, nav } from "@/hooks/useRoute"
import type { Project } from "@/types"

// DefsPanel lists the project's roles; Start opens a fresh pane for one.
export function DefsPanel({ proj, dir, className }: { proj?: Project; dir: string; className?: string }) {
  const roles = proj?.roles ?? []
  const start = (role: string) => act(post<{ pane: string }>("/api/agents", {
    Agent: "", Model: "", Cwd: dir, Role: role, Name: "", Prompt: "", Worktree: false,
  }).then((r) => nav("/agent/" + enc(r.pane))))
  return <Panel className={className} icon={<Users className="text-blue-600" />} title="Roles" right={<span>{roles.length}</span>} bodyClass="py-1">
    {!roles.length && <p className="px-2.5 py-2 text-xs text-muted-foreground">No roles in this project.</p>}
    {roles.map((role) => <div key={role} className="group flex h-8 items-center gap-2 px-2.5 text-[12.5px] hover:bg-accent">
      <Avatar role={role} />
      <span className="min-w-0 flex-1 truncate">{role}</span>
      <button type="button" aria-label={`Start ${role}`} title={`Start a ${role} agent`} className="flex size-6 items-center justify-center rounded-md text-muted-foreground opacity-60 group-hover:opacity-100 hover:bg-emerald-50 hover:text-emerald-700" onClick={() => void start(role)}><Play className="size-3.5 fill-current" /></button>
    </div>)}
  </Panel>
}
