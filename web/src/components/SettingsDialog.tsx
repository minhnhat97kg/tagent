import { getDisplayDefault, setDisplayDefault, type DisplayDefault } from "@/lib/transcript"
import { Lock, Plus, RotateCcw } from "lucide-react"
import { toast } from "sonner"
import { api, post } from "@/api"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog"
import { Switch } from "@/components/ui/switch"
import { usePoll } from "@/hooks/usePoll"
import { baseName } from "@/lib/status"
import { useAppState } from "@/state"

interface Setting { key: string; group: string; label: string; help: string; kind: "bool" | "choice" | "preset" | "int" | "text"; value: string; default: string; choices: string[] | null; min: number; max: number; readOnly: boolean; secret: boolean; reload: boolean }

// SettingsDialog edits the same registry as `tagent config` and the TUI settings screen (,).
export function SettingsDialog({ open, onOpenChange, onResetLayout, onAddWorkspace }: { open: boolean; onOpenChange: (open: boolean) => void; onResetLayout: () => void; onAddWorkspace: () => void }) {
  const poll = usePoll<Setting[]>(() => (open ? api<Setting[]>("/api/config") : Promise.resolve([])), 5000, String(open))
  const save = (s: Setting, body: { Value?: string; Reset?: boolean }) => {
    if (!body.Reset && body.Value === s.value) return
    post("/api/config", { Key: s.key, ...body }).then(() => {
      poll.reload(); toast.success(`${s.label} saved${s.reload ? " (reloaded keys)" : ""}`)
    }, e => toast.error((e as Error).message))
  }
  const { state } = useAppState()
  const removeWorkspace = (dir: string) => api(`/api/workspaces?dir=${encodeURIComponent(dir)}`, { method: "DELETE" }).then(
    () => toast.success(`Removed ${baseName(dir)}`), e => toast.error((e as Error).message))
  // "Model for new" belongs to the default agent: offer that harness's own models.
  const defaultAgent = (poll.data ?? []).find(x => x.key === "@agent-sidebar-default-agent")?.value
  const groups = [...new Set((poll.data ?? []).map(s => s.group))]
  const control = (s: Setting) => {
    const cls = "h-9 w-full rounded-lg border bg-card px-2.5 text-[12.5px] outline-none focus:border-brand focus:ring-3 focus:ring-brand/15 disabled:opacity-60"
    if (s.readOnly) return <span className="flex items-center gap-1.5 text-[12.5px] text-muted-foreground"><Lock className="size-3" />{s.secret ? (s.value ? "set" : "not set") : s.value || "—"} · change in the TUI</span>
    if (s.kind === "bool") return <Switch checked={s.value === "on"} onCheckedChange={v => save(s, { Value: v ? "on" : "off" })} />
    if (s.kind === "choice") return <select className={cls} value={s.value} onChange={e => save(s, { Value: e.target.value })}>{(s.choices ?? []).map(c => <option key={c}>{c}</option>)}</select>
    if (s.key === "@agent_model" && (state?.models?.[defaultAgent ?? ""] ?? []).length) {
      const ms = state!.models![defaultAgent!]
      return <select className={cls} value={s.value} onChange={e => save(s, { Value: e.target.value })}>
        <option value="">default model</option>
        {(ms.includes(s.value) || !s.value ? ms : [s.value, ...ms]).map(m => <option key={m} value={m}>{m}</option>)}
      </select>
    }
    // A preset with a closed list of real options (the harnesses) is picked, not typed.
    if (s.key === "@agent-sidebar-default-agent") return <select className={cls} value={s.value} onChange={e => save(s, { Value: e.target.value })}>{(s.choices ?? []).map(c => <option key={c}>{c}</option>)}</select>
    const list = s.kind === "preset" ? `choices-${s.key}` : undefined
    return <>
      <input key={s.value} className={cls} list={list} type={s.kind === "int" ? "number" : "text"} min={s.min || undefined} max={s.max || undefined} defaultValue={s.value} placeholder={s.default}
        onBlur={e => save(s, { Value: e.target.value })} onKeyDown={e => { if (e.key === "Enter") e.currentTarget.blur() }} />
      {list && <datalist id={list}>{(s.choices ?? []).map(c => <option key={c} value={c} />)}</datalist>}
    </>
  }
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="board-drawer translate-x-0 translate-y-0 sm:max-w-none" onOpenAutoFocus={e => { e.preventDefault(); (e.currentTarget as HTMLElement).focus() }}>
      <DialogTitle className="drawer-title !mt-0">Settings</DialogTitle>
      <DialogDescription className="board-muted">Saved to tmux and your tagent config file at once. Same options as <code>tagent config</code>.</DialogDescription>
      {!poll.data && <p className="board-muted">Loading…</p>}
      {groups.map(g => <section key={g}>
        <h3>{g}</h3>
        <div className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border bg-muted/50">{poll.data!.filter(s => s.group === g).map(s => <div key={s.key} className="grid grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)_24px] items-center gap-4 px-4 py-3">
          <div className="min-w-0"><div className="text-[12.5px] font-medium">{s.label}</div>{s.help && <div className="text-[11px] leading-snug text-muted-foreground">{s.help}</div>}</div>
          <div className="min-w-0">{control(s)}</div>
          {!s.readOnly && s.value !== s.default && !s.secret ? <button type="button" aria-label={`Reset ${s.label}`} title={`Reset to ${s.default || "empty"}`} className="flex size-6 items-center justify-center rounded-md text-muted-foreground hover:bg-accent" onClick={() => save(s, { Reset: true })}><RotateCcw className="size-3.5" /></button> : <span />}
        </div>)}</div>
      </section>)}
      <section>
        <h3>Workspaces</h3>
        <div className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border bg-muted/50">
          {(state?.projects ?? []).map(p => <div key={p} className="flex items-center gap-3 px-4 py-3">
            <div className="min-w-0 flex-1"><div className="text-[12.5px] font-medium">{baseName(p)}</div><div className="truncate text-[11px] text-muted-foreground" title={p}>{p}</div></div>
            <Button variant="ghost" size="sm" aria-label={`Remove ${baseName(p)}`} onClick={() => removeWorkspace(p)}>Remove</Button>
          </div>)}
          <div className="px-4 py-3"><Button variant="outline" size="sm" onClick={onAddWorkspace}><Plus /> Add workspace…</Button></div>
        </div>
      </section>
      <section>
        <h3>This device</h3>
        <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)_24px] items-center gap-4 rounded-xl border bg-muted/50 px-4 py-3">
          <div className="min-w-0"><div className="text-[12.5px] font-medium">Agent display</div><div className="text-[11px] leading-snug text-muted-foreground">Default view for agents; the toggle beside Handoff overrides it per agent</div></div>
          <select aria-label="Agent display" className="h-9 w-full rounded-lg border bg-card px-2.5 text-[12.5px]" defaultValue={getDisplayDefault()} onChange={e => setDisplayDefault(e.target.value as DisplayDefault)}>
            <option value="auto">Auto (Chat on phone)</option><option value="terminal">Terminal</option><option value="chat">Chat</option>
          </select><span />
        </div>
        <div className="mt-2 grid grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)_24px] items-center gap-4 rounded-xl border bg-muted/50 px-4 py-3">
          <div className="min-w-0"><div className="text-[12.5px] font-medium">Panel layout</div><div className="text-[11px] leading-snug text-muted-foreground">Drag the dividers; saved on this device</div></div>
          <button type="button" className="h-9 rounded-lg border bg-card px-2.5 text-[12.5px] hover:bg-accent" onClick={onResetLayout}>Reset layout</button><span />
        </div>
      </section>
    </DialogContent>
  </Dialog>
}
