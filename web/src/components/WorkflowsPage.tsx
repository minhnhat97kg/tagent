import { useEffect, useRef, useState } from 'react'
import { ArrowDown, ArrowLeft, Bot, CheckCircle2, CircleDot, Play, Plus, Save, Terminal, Trash2, Workflow as WorkflowIcon } from 'lucide-react'
import { api, send } from '@/api'
import { usePoll } from '@/hooks/usePoll'
import { enc } from '@/hooks/useRoute'
import { Button } from '@/components/ui/button'
import { statusColor } from '@/components/BoardDrawer'

type Job = { ID: string; Status: string; Agent: string; Model: string; Prompt: string; Run: string; Outputs: string[]; OnFailure: string; MaxLoops: number }
type Input = { Description: string; Default: string; Required: boolean }
type Flow = { key: string; name: string; global: boolean; yaml: string; description: string; error: string; jobs: Job[]; inputs: Record<string, Input>; total: number; running: number; lastUsed: string }
type Preview = { Name: string; Description: string; Jobs: Job[] }

export function WorkflowsPage({ dir }: { dir: string }) {
  return <WorkflowEditor key={dir} dir={dir} />
}

// FlowChart draws the recipe as it would run: ticket → each step in order → done, with failure routes.
function FlowChart({ jobs }: { jobs: Job[] }) {
  const arrow = <ArrowDown className="mx-auto my-1 size-4 text-muted-foreground" />
  return <div className="flex flex-col items-stretch">
    <div className="mx-auto flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs text-muted-foreground"><CircleDot className="size-3.5" />New ticket</div>
    {jobs.map(j => <div key={j.ID}>
      {arrow}
      <details className="group rounded-lg border bg-card shadow-sm" style={{ borderLeft: `3px solid ${statusColor(j.Status)}` }}>
        <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2">
          {j.Run ? <Terminal className="size-4 shrink-0 text-sky-600" /> : <Bot className="size-4 shrink-0 text-indigo-600" />}
          <span className="font-medium">{j.ID}</span>
          <span className="rounded px-1.5 text-[11px]" style={{ color: statusColor(j.Status), background: 'var(--muted)' }}>{j.Status}</span>
          <span className="ml-auto truncate text-xs text-muted-foreground">{j.Run ? 'command' : j.Agent}{j.Model ? ` · ${j.Model}` : ''}</span>
        </summary>
        <div className="space-y-1 border-t px-3 py-2 text-xs">
          <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono">{j.Run || j.Prompt || 'Uses the role prompt'}</pre>
          {!!j.Outputs?.length && <p className="text-muted-foreground">Outputs: {j.Outputs.join(', ')}</p>}
        </div>
      </details>
      {(j.OnFailure || j.Run || j.MaxLoops > 0) && <p className="mt-1 text-right text-[11px] text-muted-foreground">
        {j.MaxLoops > 0 && `max ${j.MaxLoops} loops · `}fail → {j.OnFailure || 'blocked'}</p>}
    </div>)}
    {arrow}
    <div className="mx-auto flex items-center gap-1.5 rounded-full border border-emerald-500/40 px-3 py-1 text-xs text-emerald-600"><CheckCircle2 className="size-3.5" />done</div>
  </div>
}

function WorkflowEditor({ dir }: { dir: string }) {
  const path = '/api/workflows?dir=' + enc(dir)
  const poll = usePoll(() => api<Flow[]>(path), 2000, dir)
  const [selected, select] = useState<Flow>()
  const [yaml, setYAML] = useState('')
  const [preview, setPreview] = useState<Preview>()
  const [validation, setValidation] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [run, setRun] = useState<Flow>()
  const [values, setValues] = useState<Record<string, string>>({})
  const revision = useRef(0)
  // Re-parse the YAML as it is typed so the chart follows the text.
  useEffect(() => {
    if (!selected) return
    const id = ++revision.current
    let active = true
    const timer = setTimeout(() => {
      void send('POST', path, { Op: 'validate', YAML: yaml }).then(result => {
        if (!active || id !== revision.current) return
        setPreview((result as { preview: Preview }).preview); setValidation('')
      }).catch(e => { if (active && id === revision.current) setValidation((e as Error).message) })
    }, 250)
    return () => { active = false; clearTimeout(timer) }
  }, [yaml, selected, path])
  async function mutate(method: string, body: object) {
    setBusy(true); setError('')
    try { await send(method, path, body); poll.reload(); if (method === 'DELETE') select(undefined); return true }
    catch (e) { setError((e as Error).message); return false }
    finally { setBusy(false) }
  }
  const dirty = !!selected && yaml !== selected.yaml
  function open(f?: Flow) {
    if (dirty && !confirm('Discard unsaved workflow changes?')) return
    select(f); setYAML(f?.yaml ?? ''); setPreview(f && { Name: f.name, Description: f.description, Jobs: f.jobs }); setError(''); setValidation(f?.error ?? '')
  }
  function start(f: Flow) { setRun(f); setError(''); setValues(Object.fromEntries(Object.entries(f.inputs ?? {}).map(([key, value]) => [key, value.Default]))) }
  const flows = poll.data ?? []
  const fieldClass = 'w-full rounded border bg-background p-2'

  return <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border bg-card">
    <header className="flex h-10 shrink-0 items-center gap-2 border-b px-3">
      {selected && <Button variant="ghost" size="icon-xs" className="md:hidden" aria-label="Back to workflows" onClick={() => open(undefined)}><ArrowLeft /></Button>}
      <WorkflowIcon className="size-4 text-teal-600" /><h2 className="text-[13px] font-semibold">Workflows</h2>
      <Button size="xs" variant="outline" className="ml-auto" disabled={busy} onClick={() => { const name = prompt('Workflow filename (letters, numbers and hyphens)'); if (name) void mutate('POST', { Name: name, Op: 'new' }) }}><Plus />New</Button>
    </header>
    <div className="flex min-h-0 flex-1">
      <nav aria-label="Workflow list" className={`min-h-0 w-full shrink-0 overflow-auto border-r md:w-60 ${selected ? 'hidden md:block' : ''}`}>
        {poll.error ? <p role="alert" className="p-3 text-sm">Couldn't load workflows. {poll.error.message}</p> : !poll.data ? <p className="p-3 text-sm text-muted-foreground">Loading…</p> : !flows.length ? <p className="p-3 text-sm text-muted-foreground">No workflows yet.</p> : null}
        {flows.map(f => <button key={f.key + f.global} onClick={() => open(f)} aria-current={selected?.key === f.key && selected.global === f.global}
          className={`block w-full border-b px-3 py-2 text-left hover:bg-accent ${selected?.key === f.key && selected.global === f.global ? 'bg-accent' : ''}`}>
          <div className="flex items-center gap-1.5 text-[13px] font-medium">{f.name}{f.global && <span className="text-[10.5px] font-normal text-muted-foreground">global</span>}{f.running > 0 && <span className="ml-auto text-[11px] text-emerald-600">● {f.running}</span>}</div>
          <p className="line-clamp-2 text-[11.5px] text-muted-foreground">{f.description || `${f.jobs?.length ?? 0} steps`}</p>
          <div className="mt-1 flex flex-wrap gap-1">{(f.jobs ?? []).map(j => <i key={j.ID} title={j.Status} className="h-1 w-4 rounded-full" style={{ background: statusColor(j.Status) }} />)}</div>
          {f.error && <p className="text-[11px] text-destructive">needs fixing</p>}
        </button>)}
      </nav>
      {!selected ? <div className="hidden flex-1 items-center justify-center text-sm text-muted-foreground md:flex">Select a workflow to see its YAML and flow.</div> :
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <div className="flex shrink-0 flex-wrap items-center gap-2 border-b px-3 py-1.5">
            <h3 className="mr-auto truncate text-[13px] font-semibold">{selected.name}{dirty && <span className="ml-1 text-amber-600">•</span>}<span className="ml-2 text-[11px] font-normal text-muted-foreground">{selected.total} tickets · {selected.running} in progress</span></h3>
            <Button size="xs" disabled={busy || !!selected.error} onClick={() => start(selected)}><Play />Run</Button>
            {!selected.global && <>
              <Button size="xs" variant="outline" disabled={busy || !dirty || !!validation} onClick={async () => { if (await mutate('POST', { Name: selected.key, YAML: yaml })) select({ ...selected, yaml }) }}><Save />Save</Button>
              <Button size="xs" variant="ghost" aria-label="Delete workflow" disabled={busy} onClick={() => { if (confirm('Delete workflow?')) void mutate('DELETE', { Name: selected.key }) }}><Trash2 /></Button>
            </>}
          </div>
          {(validation || error) && <p role="alert" className="shrink-0 whitespace-pre-wrap border-b bg-destructive/5 px-3 py-1.5 text-xs text-destructive">{validation || error}</p>}
          <div className="grid min-h-0 flex-1 grid-rows-2 md:grid-cols-2 md:grid-rows-1">
            <div className="flex min-h-0 flex-col border-b md:border-b-0 md:border-r">
              <div className="shrink-0 px-3 pt-2 text-[11px] font-medium text-muted-foreground">YAML{selected.global && ' · global, read-only'}</div>
              <textarea aria-label="Workflow YAML" spellCheck={false} readOnly={selected.global} value={yaml} onChange={e => setYAML(e.target.value)}
                className="min-h-0 flex-1 resize-none bg-transparent p-3 font-mono text-[12px] leading-5 outline-none" />
            </div>
            <div className="min-h-0 overflow-auto p-3">
              <div className="mb-2 text-[11px] font-medium text-muted-foreground">Flow{validation && ' · last valid version'}</div>
              <FlowChart jobs={preview?.Jobs ?? selected.jobs ?? []} />
            </div>
          </div>
        </div>}
    </div>
    {run && <div role="dialog" aria-modal="true" aria-label={'Run ' + run.name} className="fixed inset-0 z-50 overflow-auto bg-background/95 p-6"><form className="mx-auto max-w-lg space-y-4" onSubmit={async e => { e.preventDefault(); if (await mutate('POST', { Name: run.key, Global: run.global, Op: 'run', Inputs: values })) setRun(undefined) }}>
      <h3 className="font-semibold">Run {run.name}</h3><p className="text-sm text-muted-foreground">This creates a new ticket and starts its first step.</p>
      {Object.entries(run.inputs ?? {}).map(([key, input]) => <label key={key} className="block">{key} {input.Required ? '(required)' : '(optional)'}<p className="text-sm text-muted-foreground">{input.Description}</p><textarea autoFocus required={input.Required} className={fieldClass} value={values[key] ?? ''} onChange={e => setValues({ ...values, [key]: e.target.value })} /></label>)}
      {!Object.keys(run.inputs ?? {}).length && <p>No inputs needed.</p>}
      {error && <p role="alert" className="text-destructive">{error}</p>}<Button type="submit" disabled={busy}>Run</Button> <Button type="button" variant="outline" disabled={busy} onClick={() => setRun(undefined)}>Cancel</Button>
    </form></div>}
  </section>
}
