import { Fragment, useId, useRef, useState, type FocusEvent, type FormEvent } from "react"
import { ArrowDown, ArrowRight, ArrowUp, GripVertical, Plus, Trash2 } from "lucide-react"
import { api, send } from "@/api"
import { Meter } from "@/components/panels/OverviewPanel"
import { Button } from "@/components/ui/button"
import { usePoll } from "@/hooks/usePoll"
import { enc } from "@/hooks/useRoute"
import { useAppState } from "@/state"
import { candidateName, flow, parseRef, reorder, spillReset, withUntil } from "@/lib/routing"
import type { RoutingTier, RoutingView } from "@/types"

type RoleRow = RoutingView["roles"][number]

// The shape PUT /api/routing takes: the whole file, so each save writes the current config plus one change.
type Config = { limits: Record<string, number>; when_exhausted: string; tiers: Record<string, string[]> }

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
          {!view.tiers.length && <p className="text-sm text-muted-foreground">No tiers yet. Create one below.</p>}
          <div className="grid grid-cols-1 gap-3 xl:grid-cols-2">
            {view.tiers.map(t => <TierCard key={t.name} tier={t} view={view} busy={busy} models={state?.models ?? null}
              setRefs={refs => setTier(t.name, refs)} onDelete={() => deleteTier(t.name, t.usedBy)} />)}
          </div>
          <section aria-label="New tier" className="rounded-lg border border-dashed p-3">
            <h3 className="mb-2 text-[13px] font-semibold">New tier</h3>
            <CandidateForm label="Create tier" withName disabled={busy} harnesses={view.harnesses} models={state?.models ?? null} onSubmit={createTier} />
          </section>
          <section aria-label="Parallel agents" className="flex flex-col gap-2 rounded-lg border p-3">
            <h3 className="text-[13px] font-semibold">Parallel agents</h3>
            <p className="text-[11.5px] text-muted-foreground">Most agents of each role that may run at once on this project. Empty or 0 means no limit; a ticket over the limit waits.</p>
            {!view.roles.length && <p className="text-sm text-muted-foreground">No roles in this project yet.</p>}
            {view.roles.map(r => <RoleLimit key={r.name} dir={dir} role={r} disabled={busy} onSaved={poll.reload} />)}
          </section>
          <details className="rounded-lg border p-3 text-[12px]">
            <summary className="cursor-pointer text-[13px] font-semibold">Defaults</summary>
            <div className="mt-3 flex flex-col gap-2">
              {view.providers.map(p => <label key={p.id} className="flex flex-wrap items-center gap-2">
                <span className="min-w-24 font-medium">{p.name}{p.stale && <span className="font-normal text-muted-foreground"> · stale</span>}</span>
                <span className="text-muted-foreground">Exhausted at</span>
                <Threshold key={view.limits[p.id]} label={`${p.name} limit`} value={view.limits[p.id]} disabled={busy} onSave={n => void save(c => ({ ...c, limits: { ...c.limits, [p.id]: n } }))} />
                <span className="text-muted-foreground">%</span>
              </label>)}
              <label className="flex flex-wrap items-center gap-2 font-medium">When every candidate is exhausted, in all tiers
                <select aria-label="When exhausted" className={fieldClass} disabled={busy} value={view.when_exhausted}
                  onChange={e => { const v = e.target.value; void save(c => ({ ...c, when_exhausted: v })) }}>
                  <option value="block">block: the ticket moves to blocked</option>
                  <option value="first">first: run the first candidate</option>
                </select></label>
            </div>
          </details>
        </>}
    </div>
  </section>
}

// TierCard is one tier: a node chart of its flow, candidates in priority order (drag, buttons or remove), and the add form.
// The flow text stays in the DOM for screen readers; the chart is decorative.
function TierCard({ tier: t, view, busy, models, setRefs, onDelete }: {
  tier: RoutingTier; view: RoutingView; busy: boolean; models: Record<string, string[]> | null
  setRefs: (refs: string[]) => Promise<boolean>; onDelete: () => void
}) {
  const refs = t.candidates.map(c => c.ref)
  const dragFrom = useRef<number | null>(null)
  const move = (from: number, to: number) => { if (!busy && from !== to) void setRefs(reorder(refs, from, to)) }
  return <section aria-label={`Tier ${t.name}`} className="flex flex-col gap-2 rounded-lg border p-3">
    <header className="flex items-center gap-2">
      <h3 className="text-[13px] font-semibold">{t.name}</h3>
      <span className="min-w-0 truncate text-[11.5px] text-muted-foreground">used by {t.usedBy.length ? t.usedBy.join(", ") : "no role in this project"}</span>
      <Button size="icon-xs" variant="ghost" className="ml-auto" aria-label={`Delete tier ${t.name}`} disabled={busy} onClick={onDelete}><Trash2 /></Button>
    </header>
    <p className="sr-only">{flow(t.candidates, view.when_exhausted)}</p>
    <div aria-hidden className="flex flex-wrap items-center gap-1 text-[11px]">
      {t.candidates.map((c, i) => <Fragment key={`${i}-${c.ref}`}>
        {i > 0 && <ArrowRight className="size-3 shrink-0 text-muted-foreground" />}
        <span className={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 font-medium ${c.selected ? "border-brand bg-brand-soft text-brand-ink" : "bg-background"}`}>
          <span className="size-1.5 rounded-full bg-current opacity-70" />
          {candidateName(c.ref)}
          {c.threshold !== null && <span className="font-normal text-muted-foreground">until {c.threshold}%</span>}
        </span>
      </Fragment>)}
      <ArrowRight className="size-3 shrink-0 text-muted-foreground" />
      <span className={`rounded-full border border-dashed px-2 py-0.5 ${view.when_exhausted === "first" ? "text-brand-ink" : "text-destructive"}`}>{view.when_exhausted === "first" ? "run first" : "block"}</span>
    </div>
    <ol className="relative flex flex-col gap-1.5 border-l-2 border-dashed border-border pl-3 ml-1.5">
      {t.candidates.map((c, i) => {
        const provider = view.providers.find(p => p.id === parseRef(c.ref).harness)
        return <li key={`${i}-${c.ref}`} aria-label={c.ref}
          onDragOver={e => { if (dragFrom.current !== null) e.preventDefault() }}
          onDrop={e => { e.preventDefault(); if (dragFrom.current !== null) move(dragFrom.current, i); dragFrom.current = null }}
          className={`rounded-md border px-2.5 py-2 ${c.selected ? "border-brand bg-brand-soft/40" : ""}`}>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1.5 text-[12.5px]">
            <span draggable={!busy} aria-label={`Drag ${c.ref}`} title="Drag to reorder"
              onDragStart={e => { dragFrom.current = i; e.dataTransfer?.setData("text/plain", c.ref) }}
              onDragEnd={() => { dragFrom.current = null }}
              className="flex cursor-grab text-muted-foreground active:cursor-grabbing"><GripVertical className="size-3.5" /></span>
            <span className="w-3 text-center text-[11px] tabular-nums text-muted-foreground">{i + 1}</span>
            <span className="font-medium">{candidateName(c.ref)}</span>
            {c.selected ? <span className="rounded bg-brand-soft px-1.5 text-[11px] font-medium text-brand-ink">Running now</span>
              : c.spilled ? <span className="rounded bg-destructive/10 px-1.5 text-[11px] text-destructive">{`Spilled - ${resetText(spillReset(provider, c.threshold))}`}</span>
              : <span className="text-[11px] text-muted-foreground">{c.state}{c.why ? ` · ${c.why}` : ""}</span>}
            <span className="ml-auto flex gap-0.5">
              <Button size="icon-xs" variant="ghost" aria-label={`Move ${c.ref} up`} disabled={busy || i === 0} onClick={() => move(i, i - 1)}><ArrowUp /></Button>
              <Button size="icon-xs" variant="ghost" aria-label={`Move ${c.ref} down`} disabled={busy || i === refs.length - 1} onClick={() => move(i, i + 1)}><ArrowDown /></Button>
              <Button size="icon-xs" variant="ghost" aria-label={`Remove ${c.ref}`} disabled={busy || refs.length < 2} onClick={() => void setRefs(refs.filter((_, j) => j !== i))}><Trash2 /></Button>
            </span>
          </div>
          <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1.5 pl-9">
            {c.threshold === null ? <span className="text-[11.5px] text-muted-foreground">no quota</span> : <>
              {provider && <div className="flex min-w-44 flex-1 flex-col gap-1">
                <Meter label="5h" value={provider.five.usedPercent} marker={c.threshold} />
                <Meter label="7d" value={provider.week.usedPercent} marker={c.threshold} />
              </div>}
              <label className="flex items-center gap-1 text-[11.5px] text-muted-foreground">Spill at
                <input type="number" min={1} max={100} aria-label={`${c.ref} spill at`} placeholder={String(c.threshold)} defaultValue={c.until ?? ""} key={`${c.ref}-${c.until}`} disabled={busy}
                  className="w-14 rounded border bg-background p-1 text-right text-[12px] tabular-nums text-foreground disabled:opacity-60"
                  onBlur={e => {
                    const text = e.currentTarget.value.trim()
                    if (text === (c.until === null ? "" : String(c.until))) return
                    void setRefs(refs.map((r, j) => j === i ? withUntil(r, text) : r))
                  }}
                  onKeyDown={e => { if (e.key === "Enter") e.currentTarget.blur() }} />%
              </label>
            </>}
          </div>
        </li>
      })}
    </ol>
    {t.reason && <p className="text-[11.5px] text-brand-ink">{t.selected ? `Skipped for quota: ${t.reason}` : t.reason}</p>}
    <div><CandidateForm label="Add candidate" disabled={busy} harnesses={view.harnesses} models={models} onSubmit={(_, ref) => setRefs([...refs, ref])} /></div>
  </section>
}

// Harness, model and an optional name: a candidate to add, or the first candidate of a new tier.
// onSubmit returns whether it saved; a saved form clears itself.
function CandidateForm({ label, withName, disabled, harnesses, models, onSubmit }: {
  label: string
  withName?: boolean
  disabled: boolean
  harnesses: string[]
  models: Record<string, string[]> | null
  onSubmit: (name: string, ref: string) => Promise<boolean>
}) {
  const [name, setName] = useState("")
  const [harness, setHarness] = useState(harnesses[0] ?? "")
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

// RoleLimit edits one role's max_instances; it saves on blur or Enter, only when changed, and shows the server's error beside the field.
function RoleLimit({ dir, role, disabled, onSaved }: { dir: string; role: RoleRow; disabled: boolean; onSaved: () => Promise<unknown> }) {
  const [error, setError] = useState("")
  const current = role.maxInstances
  async function save(e: FocusEvent<HTMLInputElement>) {
    const input = e.currentTarget
    const text = input.value.trim()
    if (input.validity.badInput || (text !== "" && !/^\d+$/.test(text))) { setError("Enter a whole number, 0 or more"); return }
    const max = text === "" ? 0 : Number(text)
    if (max === current) { setError(""); return }
    try {
      await send("PUT", "/api/board/config/role-limit?dir=" + enc(dir), { role: role.name, max })
      setError("")
      await onSaved()
    } catch (err) {
      setError((err as Error).message)
    }
  }
  return <div className="flex flex-col gap-1 text-[12px]">
    <label className="flex flex-wrap items-center gap-2">
      <span className="min-w-28 font-medium">{role.name}{role.tier && <span className="font-normal text-muted-foreground"> · {role.tier}</span>}</span>
      <input type="number" min={0} step={1} aria-label={`${role.name} parallel agents`} placeholder="no limit"
        defaultValue={current || ""} key={current} disabled={disabled}
        className="w-20 rounded border bg-background p-1 text-right tabular-nums disabled:opacity-60"
        onBlur={save} onKeyDown={e => { if (e.key === "Enter") e.currentTarget.blur() }} />
    </label>
    {error && <p role="alert" className="text-destructive">{error}</p>}
  </div>
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
