import { useId, useState, type FormEvent } from "react"
import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react"
import { api, send } from "@/api"
import { Meter } from "@/components/panels/OverviewPanel"
import { Button } from "@/components/ui/button"
import { usePoll } from "@/hooks/usePoll"
import { enc } from "@/hooks/useRoute"
import { useAppState } from "@/state"
import type { RoutingView } from "@/types"

// The shape PUT /api/routing takes: the whole file, so each save writes the current config plus one change.
type Config = { limits: Record<string, number>; when_exhausted: string; tiers: Record<string, string[]> }

const harnesses = ["claude", "pi", "codex"]
const fieldClass = "rounded border bg-background p-2 text-sm disabled:opacity-60"
const configOf = (v: RoutingView): Config => ({
  limits: v.limits,
  when_exhausted: v.when_exhausted,
  tiers: Object.fromEntries(v.tiers.map(t => [t.name, t.candidates.map(c => c.ref)])),
})
const resetText = (at: string | null) => at ? `resets ${new Date(at).toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}` : "no reset time"

// RoutingPage edits the account's routing.yaml. Every action saves at once; there is no draft,
// so the selected candidate follows the quota poll. The server computes state and selection.
export function RoutingPage({ dir }: { dir: string }) {
  const path = "/api/routing?dir=" + enc(dir)
  const poll = usePoll(() => api<RoutingView>(path), 3000, dir)
  const { state } = useAppState()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const view = poll.data

  async function save(change: (c: Config) => Config): Promise<boolean> {
    if (!view) return false
    setBusy(true); setError("")
    try {
      await send("PUT", path, change(configOf(view)))
      await poll.reload()
      return true
    } catch (e) {
      setError((e as Error).message)
      return false
    } finally {
      setBusy(false)
    }
  }
  const setTier = (name: string, refs: string[]) => save(c => ({ ...c, tiers: { ...c.tiers, [name]: refs } }))
  function deleteTier(name: string, usedBy: string[]) {
    const who = usedBy.length ? ` ${usedBy.join(", ")} in this project will block at dispatch.` : ""
    if (!confirm(`Delete tier ${name}?${who} Other projects may use it too.`)) return
    void save(c => { const tiers = { ...c.tiers }; delete tiers[name]; return { ...c, tiers } })
  }
  function createTier(name: string, ref: string) {
    if (!view || view.tiers.some(t => t.name === name)) { setError(`Tier ${name} already exists`); return Promise.resolve(false) }
    return save(c => ({ ...c, tiers: { ...c.tiers, [name]: [ref] } }))
  }

  return <section className="flex h-full min-h-0 flex-col overflow-auto rounded-lg border bg-card">
    <header className="flex h-10 shrink-0 items-center gap-2 border-b px-3">
      <h2 className="text-[13px] font-semibold">Routing</h2>
      {view && <span className="min-w-0 truncate text-[11px] text-muted-foreground">{view.path}</span>}
    </header>
    <div className="flex flex-col gap-3 p-3">
      {(error || poll.error) && <p role="alert" className="whitespace-pre-wrap rounded border border-destructive/30 bg-destructive/5 p-2 text-xs text-destructive">{error || `Couldn't load routing. ${poll.error?.message}`}</p>}
      {!view ? <p className="text-sm text-muted-foreground">Loading…</p>
        : view.error ? <p role="alert" className="text-sm">{view.path} does not load, so it cannot be edited here: {view.error}</p>
        : <>
          <p className="text-[11.5px] text-muted-foreground">Tiers are account-wide: every project uses this file. Roles name a tier on the Agents page.</p>
          <section aria-label="Quota" className="grid gap-3 rounded-lg border p-3 sm:grid-cols-2">
            {view.providers.map(p => <div key={p.id} className="flex flex-col gap-1">
              <div className="flex items-center gap-1.5 text-[12px] font-medium">{p.name}{p.stale && <span className="text-[10.5px] font-normal text-muted-foreground">· stale</span>}
                <label className="ml-auto flex items-center gap-1 font-normal text-muted-foreground">Exhausted at
                  <Threshold key={view.limits[p.id]} label={`${p.name} limit`} value={view.limits[p.id]} disabled={busy} onSave={n => void save(c => ({ ...c, limits: { ...c.limits, [p.id]: n } }))} />%</label></div>
              <div className="flex gap-3"><Meter label="5h" value={p.five.usedPercent} /><Meter label="7d" value={p.week.usedPercent} /></div>
              <p className="text-[11px] text-muted-foreground">5h {resetText(p.five.resetsAt)} · 7d {resetText(p.week.resetsAt)}</p>
            </div>)}
          </section>
          <label className="text-[12px] font-medium">When every candidate is exhausted
            <select aria-label="When exhausted" className={`${fieldClass} ml-2`} disabled={busy} value={view.when_exhausted}
              onChange={e => { const v = e.target.value; void save(c => ({ ...c, when_exhausted: v })) }}>
              <option value="block">block: the ticket moves to blocked</option>
              <option value="first">first: run the first candidate</option>
            </select></label>
          {!view.tiers.length && <p className="text-sm text-muted-foreground">No tiers yet. Create one below.</p>}
          {view.tiers.map(t => {
            const refs = t.candidates.map(c => c.ref)
            return <section key={t.name} aria-label={`Tier ${t.name}`} className="rounded-lg border p-3">
              <header className="flex items-center gap-2">
                <h3 className="text-[13px] font-semibold">{t.name}</h3>
                <span className="text-[11.5px] text-muted-foreground">used by {t.usedBy.length ? t.usedBy.join(", ") : "no role in this project"}</span>
                <Button size="icon-xs" variant="ghost" className="ml-auto" aria-label={`Delete tier ${t.name}`} disabled={busy} onClick={() => deleteTier(t.name, t.usedBy)}><Trash2 /></Button>
              </header>
              <ol className="mt-2 space-y-1">
                {t.candidates.map((c, i) => <li key={`${i}-${c.ref}`} className="flex flex-wrap items-center gap-2 text-[12.5px]">
                  <span className="font-mono">{c.ref}</span>
                  <span className="text-muted-foreground">{c.state}{c.why ? ` · ${c.why}` : ""}</span>
                  {c.threshold !== null && <label className="flex items-center gap-1 text-muted-foreground">Until %
                    <input type="number" min={1} max={100} aria-label={`${c.ref} threshold`} placeholder={String(c.threshold)} defaultValue={c.until ?? ""} key={c.ref} disabled={busy}
                      className="w-14 rounded border bg-background p-1 text-right text-[12px] tabular-nums disabled:opacity-60"
                      onBlur={e => {
                        const text = e.currentTarget.value.trim()
                        if (text === (c.until === null ? "" : String(c.until))) return
                        const base = c.until === null ? c.ref : c.ref.slice(0, c.ref.lastIndexOf("@"))
                        void setTier(t.name, refs.map((r, j) => j === i ? (text === "" ? base : `${base}@${text}`) : r))
                      }}
                      onKeyDown={e => { if (e.key === "Enter") e.currentTarget.blur() }} />
                  </label>}
                  {c.selected && <span className="rounded bg-emerald-100 px-1.5 text-[11px] text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-300">selected</span>}
                  <span className="ml-auto flex gap-0.5">
                    <Button size="icon-xs" variant="ghost" aria-label={`Move ${c.ref} up`} disabled={busy || i === 0} onClick={() => { const r = [...refs]; [r[i - 1], r[i]] = [r[i], r[i - 1]]; void setTier(t.name, r) }}><ArrowUp /></Button>
                    <Button size="icon-xs" variant="ghost" aria-label={`Move ${c.ref} down`} disabled={busy || i === refs.length - 1} onClick={() => { const r = [...refs]; [r[i + 1], r[i]] = [r[i], r[i + 1]]; void setTier(t.name, r) }}><ArrowDown /></Button>
                    <Button size="icon-xs" variant="ghost" aria-label={`Remove ${c.ref}`} disabled={busy || refs.length < 2} onClick={() => void setTier(t.name, refs.filter((_, j) => j !== i))}><Trash2 /></Button>
                  </span>
                </li>)}
              </ol>
              {t.reason && <p className="mt-1 text-[11.5px] text-amber-700 dark:text-amber-400">{t.selected ? `Skipped for quota: ${t.reason}` : t.reason}</p>}
              <div className="mt-2"><CandidateForm label="Add candidate" disabled={busy} models={state?.models ?? null} onSubmit={(_, ref) => setTier(t.name, [...refs, ref])} /></div>
            </section>
          })}
          <section aria-label="New tier" className="rounded-lg border border-dashed p-3">
            <h3 className="mb-2 text-[13px] font-semibold">New tier</h3>
            <CandidateForm label="Create tier" withName disabled={busy} models={state?.models ?? null} onSubmit={createTier} />
          </section>
        </>}
    </div>
  </section>
}

// Harness, model and an optional name: a candidate to add, or the first candidate of a new tier.
// onSubmit returns whether it saved; a saved form clears itself.
function CandidateForm({ label, withName, disabled, models, onSubmit }: {
  label: string
  withName?: boolean
  disabled: boolean
  models: Record<string, string[]> | null
  onSubmit: (name: string, ref: string) => Promise<boolean>
}) {
  const [name, setName] = useState("")
  const [harness, setHarness] = useState("claude")
  const [model, setModel] = useState("")
  const list = models?.[harness] ?? []
  const listId = useId() // one datalist per form: every tier card has its own
  async function submit(e: FormEvent) {
    e.preventDefault()
    if (await onSubmit(name.trim(), `${harness}:${model.trim()}`)) { setName(""); setModel("") }
  }
  return <form onSubmit={submit} className="flex flex-wrap items-end gap-2 text-[12px]">
    {withName && <label className="font-medium">Tier name
      <input aria-label="Tier name" className={`${fieldClass} ml-1 w-40`} disabled={disabled} value={name} onChange={e => setName(e.target.value)} /></label>}
    <label className="font-medium">Harness
      <select aria-label={`${label} harness`} className={`${fieldClass} ml-1`} disabled={disabled} value={harness} onChange={e => setHarness(e.target.value)}>
        {harnesses.map(h => <option key={h} value={h}>{h}</option>)}
      </select></label>
    <label className="font-medium">Model
      <input aria-label={`${label} model`} list={listId} className={`${fieldClass} ml-1 w-56`} disabled={disabled} value={model} placeholder="model id[@until]" onChange={e => setModel(e.target.value)} />
      <datalist id={listId}>{list.map(m => <option key={m} value={m} />)}</datalist></label>
    <Button type="submit" size="xs" variant="outline" disabled={disabled || !model.trim() || (!!withName && !name.trim())}><Plus />{label}</Button>
  </form>
}

// Threshold is the percent used that counts as exhausted; it saves on blur or Enter, and only when changed.
function Threshold({ label, value, disabled, onSave }: { label: string; value: number; disabled: boolean; onSave: (n: number) => void }) {
  return <input type="number" min={1} max={100} aria-label={label} defaultValue={value} disabled={disabled}
    className="w-16 rounded border bg-background p-1 text-right text-[12px] tabular-nums disabled:opacity-60"
    onBlur={e => {
      const text = e.currentTarget.value.trim()
      if (text === "") { e.currentTarget.value = String(value); return } // an empty field is not 0
      const n = Number(text)
      if (n !== value) onSave(n)
    }}
    onKeyDown={e => { if (e.key === "Enter") e.currentTarget.blur() }} />
}
