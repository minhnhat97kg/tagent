import { useState } from "react"
import { Bot, GitBranch, Play } from "lucide-react"
import { toast } from "sonner"
import { api, post } from "@/api"
import { usePoll } from '@/hooks/usePoll'
import { Avatar } from './BoardDrawer'
import type { Project } from '@/types'
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { enc, nav } from "@/hooks/useRoute"
import { baseName } from "@/lib/status"
import { cn } from "@/lib/utils"
import { useAppState } from "@/state"

// NewAgentComposer picks a harness and a project, then opens the new agent's terminal: the prompt is typed there.
export function NewAgentComposer({ onDone, dir: current }: { onDone?: () => void; dir?: string }) {
  const { state } = useAppState()
  const harnesses = state?.harnesses ?? []
  const projects = state?.projects ?? []
  const [harness, setHarness] = useState(harnesses[0] ?? "claude")
  const [model, setModel] = useState("")
  const [cwd, setCwd] = useState(current || projects[0] || "")
  const roles = usePoll(() => api<Project>('/api/project?dir=' + enc(cwd)), 2000, cwd)
  const [role, setRole] = useState('')
  const [worktree, setWorktree] = useState(false)
  const [busy, setBusy] = useState(false)

  async function start() {
    setBusy(true)
    try {
      const r = await post<{ pane: string }>("/api/agents", { Agent: harness, Model: model, Cwd: cwd, Role: role, Name: "", Prompt: "", Worktree: worktree })
      onDone?.()
      nav("/agent/" + enc(r.pane))
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  const tile = (id: string, label: string, icon: React.ReactNode, hint: string) =>
    <button key={id} type="button" aria-pressed={harness === id} onClick={() => { setHarness(id); setRole('') }} onDoubleClick={() => void start()}
      className={cn("flex h-16 flex-col items-start justify-between rounded-lg border bg-card px-3 py-2 text-left transition-colors hover:border-zinc-400",
        harness === id && "border-primary ring-1 ring-primary")}>
      <span className="flex items-center gap-1.5 text-[13px] font-semibold">{icon}{label}</span>
      <span className="text-[11px] text-muted-foreground">{hint}</span>
    </button>

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {harnesses.map(h => tile(h, h, <Bot className="size-3.5 text-emerald-600" />, "terminal agent"))}
      </div>
      <section><h3 className="mb-2 text-xs text-muted-foreground">Roles</h3><div className="flex flex-wrap gap-2">{(roles.data?.roles ?? []).map(r => <button key={r} type="button" aria-pressed={role === r} className={cn('flex items-center gap-2 rounded-lg border p-3', role === r && 'ring-1 ring-primary')} onClick={() => setRole(r)}><Avatar role={r} />{r}</button>)}</div>{!roles.data?.roles?.length && <p className="text-xs text-muted-foreground">No project roles.</p>}</section>
      <div className="flex flex-wrap items-center gap-2 border-t pt-3">
        {projects.length > 0 && <select aria-label="Project" value={cwd} onChange={e => setCwd(e.target.value)} className="h-8 max-w-52 rounded-md border bg-background px-2 text-[12.5px]">
          {projects.map(p => <option key={p} value={p}>{baseName(p)}</option>)}
        </select>}
        <Input value={model} onChange={e => setModel(e.target.value)} placeholder="model (optional)" aria-label="Model" className="h-8 w-40 text-[12.5px]" />
        <label className="flex items-center gap-1.5 text-[12.5px] text-muted-foreground"><Switch checked={worktree} onCheckedChange={setWorktree} /><GitBranch className="size-3.5" />Worktree</label>
        <button type="button" disabled={busy} onClick={() => void start()} className="ml-auto flex h-8 items-center gap-1.5 rounded-md bg-primary px-3 text-[12.5px] font-medium text-primary-foreground hover:opacity-85 disabled:opacity-50">
          <Play className="size-3.5 fill-current" />Open terminal
        </button>
      </div>
      <p className="-mt-2 text-[11.5px] text-muted-foreground">The agent starts in its own terminal; type your request there.</p>
    </div>
  )
}
