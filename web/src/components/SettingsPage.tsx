import { getDisplayDefault, setDisplayDefault, type DisplayDefault } from "@/lib/transcript"
import { Lock, Plus, RotateCcw } from "lucide-react"
import { toast } from "sonner"
import { api, post } from "@/api"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { usePoll } from "@/hooks/usePoll"
import { baseName } from "@/lib/status"
import type { WebTheme } from "@/lib/theme"
import type { ReactNode } from "react"
import { useAppState } from "@/state"

interface Setting { key: string; group: string; label: string; help: string; kind: "bool" | "choice" | "preset" | "int" | "text"; value: string; default: string; choices: string[] | null; min: number; max: number; readOnly: boolean; secret: boolean; reload: boolean }

export type SettingsTab = "general" | "agents" | "routing" | "workflows" | "board" | "workspaces" | "device"

// The tabs keep each page's own hash, so older links to a page land on its tab.
const TABS: [SettingsTab, string, string][] = [
  ["general", "General", "/settings"], ["agents", "Agents", "/agents"], ["routing", "Routing", "/routing"],
  ["workflows", "Workflows", "/workflows"], ["board", "Board", "/board/config"],
  ["workspaces", "Workspaces", "/settings/workspaces"], ["device", "This device", "/settings/device"],
]

// SettingsTabs is the one place for configuration: a tab bar over the open settings page.
// unsaved asks before a tab switch drops that page's draft.
export function SettingsTabs({ tab, unsaved, children }: { tab: SettingsTab; unsaved: boolean; children: ReactNode }) {
  return <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-2">
    <nav aria-label="Settings" className="flex shrink-0 gap-1 overflow-x-auto">
      {TABS.map(([id, label, path]) => <a key={id} href={`#${path}`} aria-current={id === tab ? "page" : undefined}
        onClick={e => { if (unsaved && !confirm("Discard unsaved changes?")) e.preventDefault() }}
        className={`shrink-0 rounded-md px-2 py-1 text-[13px] ${id === tab ? "bg-accent font-medium" : "text-muted-foreground hover:text-foreground"}`}>{label}</a>)}
    </nav>
    <div className="flex min-h-0 flex-1 flex-col">{children}</div>
  </div>
}

// Card is the scrolling body of the settings tabs that have no page of their own.
function Card({ label, children }: { label: string; children: ReactNode }) {
  return <section aria-label={label} className="h-full min-h-0 overflow-auto rounded-lg border bg-card">
    <div className="flex w-full max-w-2xl flex-col gap-4 p-4 text-[13px]">{children}</div>
  </section>
}

const heading = "mb-2.5 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase"

// GeneralSettings edits the same registry as `tagent config` and the TUI settings screen (,).
export function GeneralSettings() {
  const poll = usePoll<Setting[]>(() => api<Setting[]>("/api/config"), 5000)
  const save = (s: Setting, body: { Value?: string; Reset?: boolean }) => {
    if (!body.Reset && body.Value === s.value) return
    post("/api/config", { Key: s.key, ...body }).then(() => {
      poll.reload(); toast.success(`${s.label} saved${s.reload ? " (reloaded keys)" : ""}`)
    }, e => toast.error((e as Error).message))
  }
  const { state } = useAppState()
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
  return <Card label="General settings">
      <p className="board-muted">Saved to tmux and your tagent config file at once. Same options as <code>tagent config</code>.</p>
      {!poll.data && <p className="board-muted">Loading…</p>}
      {groups.map(g => <section key={g}>
        <h3 className={heading}>{g}</h3>
        <div className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border bg-muted/50">{poll.data!.filter(s => s.group === g).map(s => <div key={s.key} className="grid grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)_24px] items-center gap-4 px-4 py-3">
          <div className="min-w-0"><div className="text-[12.5px] font-medium">{s.label}</div>{s.help && <div className="text-[11px] leading-snug text-muted-foreground">{s.help}</div>}</div>
          <div className="min-w-0">{control(s)}</div>
          {!s.readOnly && s.value !== s.default && !s.secret ? <button type="button" aria-label={`Reset ${s.label}`} title={`Reset to ${s.default || "empty"}`} className="flex size-6 items-center justify-center rounded-md text-muted-foreground hover:bg-accent" onClick={() => save(s, { Reset: true })}><RotateCcw className="size-3.5" /></button> : <span />}
        </div>)}</div>
      </section>)}
  </Card>
}

export function WorkspaceSettings({ onAddWorkspace }: { onAddWorkspace: () => void }) {
  const { state } = useAppState()
  const removeWorkspace = (dir: string) => api(`/api/workspaces?dir=${encodeURIComponent(dir)}`, { method: "DELETE" }).then(
    () => toast.success(`Removed ${baseName(dir)}`), e => toast.error((e as Error).message))
  return <Card label="Workspaces">
      <div className="flex flex-col divide-y divide-border overflow-hidden rounded-xl border bg-muted/50">
        {(state?.projects ?? []).map(p => <div key={p} className="flex items-center gap-3 px-4 py-3">
          <div className="min-w-0 flex-1"><div className="text-[12.5px] font-medium">{baseName(p)}</div><div className="truncate text-[11px] text-muted-foreground" title={p}>{p}</div></div>
          <Button variant="ghost" size="sm" aria-label={`Remove ${baseName(p)}`} onClick={() => removeWorkspace(p)}>Remove</Button>
        </div>)}
        <div className="px-4 py-3"><Button variant="outline" size="sm" onClick={onAddWorkspace}><Plus /> Add workspace…</Button></div>
      </div>
  </Card>
}

// DeviceSettings holds what is saved in this browser only.
export function DeviceSettings({ theme, onTheme, onResetLayout }: { theme: WebTheme; onTheme: (t: WebTheme) => void; onResetLayout: () => void }) {
  const row = "grid grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)] items-center gap-4 rounded-xl border bg-muted/50 px-4 py-3"
  const field = "h-9 w-full rounded-lg border bg-card px-2.5 text-[12.5px]"
  return <Card label="This device">
      <div className={row}>
        <div className="min-w-0"><div className="text-[12.5px] font-medium">Web theme</div><div className="text-[11px] leading-snug text-muted-foreground">This browser only; the TUI theme is separate</div></div>
        <select aria-label="Web theme" className={field} value={theme} onChange={e => onTheme(e.target.value as WebTheme)}>
          <option value="system">System</option><option value="light">Light</option><option value="dark">Dark</option>
        </select>
      </div>
      <div className={row}>
        <div className="min-w-0"><div className="text-[12.5px] font-medium">Agent display</div><div className="text-[11px] leading-snug text-muted-foreground">Default view for agents; the toggle beside Handoff overrides it per agent</div></div>
        <select aria-label="Agent display" className={field} defaultValue={getDisplayDefault()} onChange={e => setDisplayDefault(e.target.value as DisplayDefault)}>
          <option value="auto">Auto (Chat on phone)</option><option value="terminal">Terminal</option><option value="chat">Chat</option>
        </select>
      </div>
      <div className={row}>
        <div className="min-w-0"><div className="text-[12.5px] font-medium">Panel layout</div><div className="text-[11px] leading-snug text-muted-foreground">Drag the dividers; saved on this device</div></div>
        <button type="button" className={`${field} hover:bg-accent`} onClick={onResetLayout}>Reset layout</button>
      </div>
  </Card>
}
