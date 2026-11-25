import { Suspense, lazy, useEffect, useRef, useState } from 'react'
import { ArrowDown, ArrowLeft, Bot, CheckCircle2, CircleDot, Play, Plus, Save, Terminal, Trash2, Workflow as WorkflowIcon } from 'lucide-react'
import { api, send } from '@/api'
import { usePoll } from '@/hooks/usePoll'
import { enc } from '@/hooks/useRoute'
import { Button } from '@/components/ui/button'
import { statusColor } from '@/components/BoardDrawer'
import { jobRanges } from '@/components/workflowYaml'

const WorkflowYamlEditor = lazy(() => import('@/components/WorkflowYamlEditor'))

type Job = { ID: string; Status: string; Agent: string; Model: string; Prompt: string; Run: string; Outputs: string[]; OnFailure: string; MaxLoops: number }
type Input = { Description: string; Default: string; Required: boolean }
type Flow = { key: string; name: string; global: boolean; yaml: string; description: string; error: string; errorLine: number; jobs: Job[]; inputs: Record<string, Input>; total: number; running: number; lastUsed: string }
type Preview = { Name: string; Description: string; Jobs: Job[] }

export function WorkflowsPage({ dir, initial = '' }: { dir: string; initial?: string }) {
  return <WorkflowEditor key={dir} dir={dir} initial={initial} />
}

type Box = { top: number; bottom: number }
// FlowChart draws the recipe: start → each step → done. Failure routes back to an
// earlier step are drawn as edges on the right. A ticket's run has its own page (#/runs/<id>).
function FlowChart({ jobs, active, onSelect }: { jobs: Job[]; active: string; onSelect: (id: string) => void }) {
  const wrap = useRef<HTMLDivElement>(null)
  const cards = useRef<Record<string, HTMLElement | null>>({})
  const [boxes, setBoxes] = useState<Record<string, Box>>({})
  const [width, setWidth] = useState(0)
  useEffect(() => {
    const el = wrap.current
    if (!el) return
    const measure = () => {
      const base = el.getBoundingClientRect().top
      setWidth(el.clientWidth)
      setBoxes(Object.fromEntries(Object.entries(cards.current).filter(([, c]) => c).map(([id, c]) => { const r = c!.getBoundingClientRect(); return [id, { top: r.top - base, bottom: r.bottom - base }] })))
    }
    const ro = new ResizeObserver(measure)
    ro.observe(el); Object.values(cards.current).forEach(c => c && ro.observe(c))
    return () => ro.disconnect()
  }, [jobs])
  const index = Object.fromEntries(jobs.map((j, i) => [j.ID, i]))
  const edges = jobs.flatMap((j, i) => j.OnFailure in index && index[j.OnFailure] !== i ? [{ from: j, to: jobs[index[j.OnFailure]] }] : [])
  const gutter = edges.length ? 28 + 14 * edges.length : 0
  const arrow = (label = '') => <div className="relative my-1 flex justify-center"><ArrowDown className="size-4 text-muted-foreground" />{label && <span className="absolute left-1/2 top-0 ml-3 text-[10.5px] text-emerald-600">{label}</span>}</div>
  return <div ref={wrap} className="relative flex flex-col items-stretch" style={{ paddingRight: gutter }}>
    <div className="mx-auto flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs text-muted-foreground"><CircleDot className="size-3.5" />Start</div>
    {jobs.map((j, i) => {
      return <div key={j.ID}>
        {arrow(i > 0 && jobs[i - 1].Run ? 'pass' : '')}
        <details ref={el => { cards.current[j.ID] = el }} className={`group rounded-lg border bg-card shadow-sm ${active === j.ID ? 'ring-2 ring-sky-500' : ''}`} style={{ borderLeft: `3px solid ${statusColor(j.Status)}` }}>
          <summary onClick={() => onSelect(j.ID)} className="flex cursor-pointer list-none items-center gap-2 px-3 py-2">
            {j.Run ? <Terminal className="size-4 shrink-0 text-sky-600" /> : <Bot className="size-4 shrink-0 text-indigo-600" />}
            <span className="font-medium">{j.ID}</span>
            <span className="rounded px-1.5 text-[11px]" style={{ color: statusColor(j.Status), background: 'var(--muted)' }}>{j.Status}</span>
            <span className="ml-auto truncate text-xs text-muted-foreground">{j.Run ? 'command' : j.Agent}{j.Model ? ` · ${j.Model}` : ''}{j.MaxLoops > 0 ? ` · max ${j.MaxLoops}` : ''}</span>
          </summary>
          <div className="space-y-1 border-t px-3 py-2 text-xs">
            <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono">{j.Run || j.Prompt || 'Uses the role prompt'}</pre>
            {!!j.Outputs?.length && <p className="text-muted-foreground">Outputs: {j.Outputs.join(', ')}</p>}
          </div>
        </details>
        {!(j.OnFailure in index) && (j.OnFailure || j.Run) && <p className="mt-1 text-right text-[11px] text-muted-foreground">fail → {j.OnFailure || 'blocked'}</p>}
      </div>
    })}
    {arrow(jobs.length && jobs[jobs.length - 1].Run ? 'pass' : '')}
    <div className="mx-auto flex items-center gap-1.5 rounded-full border border-emerald-500/40 px-3 py-1 text-xs text-emerald-600"><CheckCircle2 className="size-3.5" />done</div>
    {width > 0 && <svg className="pointer-events-none absolute inset-0 overflow-visible" width={width} height="100%" aria-hidden>
      <defs><marker id="flow-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M0,0 L8,4 L0,8 z" fill="#f59e0b" /></marker></defs>
      {edges.map((e, k) => {
        const s = boxes[e.from.ID], t = boxes[e.to.ID]
        if (!s || !t) return null
        const x0 = width - gutter, x = x0 + 16 + 14 * k, y0 = s.top + 18, y1 = t.top + 18
        return <g key={e.from.ID}>
          <path d={`M${x0},${y0} H${x - 6} Q${x},${y0} ${x},${y0 + (y1 < y0 ? -6 : 6)} V${y1 + (y1 < y0 ? 6 : -6)} Q${x},${y1} ${x - 6},${y1} H${x0 + 2}`} fill="none" stroke="#f59e0b" strokeWidth="1.5" strokeDasharray="4 3" markerEnd="url(#flow-arrow)" />
          <text x={x + 7} y={(y0 + y1) / 2} fontSize="10.5" fill="#d97706" transform={`rotate(90 ${x + 7} ${(y0 + y1) / 2})`} textAnchor="middle">{e.from.Run ? 'fail' : 'changes'}{e.from.MaxLoops ? ` · max ${e.from.MaxLoops}` : ''}</text>
        </g>
      })}
    </svg>}
  </div>
}

// parseError reads the server's JSON {error, line}; older or plain-text failures have no line.
function parseError(text: string): { error: string; line: number } {
  try {
    const v = JSON.parse(text) as { error?: unknown; line?: unknown }
    if (typeof v.error === 'string') return { error: v.error, line: Number(v.line) || 0 }
  } catch { /* plain text */ }
  return { error: text, line: 0 }
}

function WorkflowEditor({ dir, initial }: { dir: string; initial: string }) {
  const path = '/api/workflows?dir=' + enc(dir)
  const poll = usePoll(() => api<Flow[]>(path), 2000, dir)
  const [selected, select] = useState<Flow>()
  const [yaml, setYAML] = useState('')
  const [preview, setPreview] = useState<Preview>()
  const [validation, setValidation] = useState('')
  const [validationLine, setValidationLine] = useState(0)
  const [goto, setGoto] = useState<{ line: number; to?: number }>()
  const [activeJob, setActiveJob] = useState('')
  const [insert, setInsert] = useState<{ text: string; n: number }>()
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
        setPreview((result as { preview: Preview }).preview); setValidation(''); setValidationLine(0)
      }).catch(e => { if (active && id === revision.current) { const { error, line } = parseError((e as Error).message); setValidation(error); setValidationLine(line) } })
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
    select(f); setYAML(f?.yaml ?? ''); setPreview(f && { Name: f.name, Description: f.description, Jobs: f.jobs }); setError(''); setValidation(f?.error ?? ''); setValidationLine(f?.errorLine ?? 0); setActiveJob(''); setGoto(undefined)
  }
  function start(f: Flow) { setRun(f); setError(''); setValues(Object.fromEntries(Object.entries(f.inputs ?? {}).map(([key, value]) => [key, value.Default]))) }
  const flows = poll.data ?? []
  // Opened with a name (#/workflows/<name>): select it once the list arrives.
  const [pending, setPending] = useState(initial)
  if (pending && poll.data) { setPending(''); const f = flows.find(x => x.name === pending || x.key === pending); if (f) open(f) }
  const fieldClass = 'w-full rounded border bg-background p-2'
  // Flow card → YAML: jump to the job's lines. YAML cursor → flow: the job under the cursor rings its card.
  function jobAt(line: number) { return Object.entries(jobRanges(yaml)).find(([, r]) => line >= r.from && line <= r.to)?.[0] ?? '' }
  function selectJob(id: string) { const r = jobRanges(yaml)[id]; setActiveJob(id); if (r) setGoto({ line: r.from, to: r.to }) }

  function newFlow() { const name = prompt('Workflow filename (letters, numbers and hyphens)'); if (name) void mutate('POST', { Name: name, Op: 'new' }) }
  return <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border bg-card">
    <header className="flex h-10 shrink-0 items-center gap-2 border-b px-3">
      {selected && <Button variant="ghost" size="icon-xs" className="md:hidden" aria-label="Back to workflows" onClick={() => open(undefined)}><ArrowLeft /></Button>}
      <WorkflowIcon className="size-4 text-teal-600" /><h2 className="text-[13px] font-semibold">Workflows</h2>
      <Button size="xs" variant="outline" className="ml-auto" disabled={busy} onClick={newFlow}><Plus />New</Button>
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
      {!selected ? <div className="hidden flex-1 flex-col items-center justify-center gap-2 text-sm text-muted-foreground md:flex">Select a workflow to see its YAML and flow.<Button size="xs" variant="outline" disabled={busy} onClick={newFlow}><Plus />New workflow</Button></div> :
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          <div className="flex shrink-0 flex-wrap items-center gap-2 border-b px-3 py-1.5">
            <h3 className="mr-auto truncate text-[13px] font-semibold">{selected.name}{dirty && <span className="ml-1 text-amber-600">•</span>}<span className="ml-2 text-[11px] font-normal text-muted-foreground">{selected.total} tickets · {selected.running} in progress</span></h3>
            <Button size="xs" disabled={busy || !!selected.error} onClick={() => start(selected)}><Play />Run</Button>
            {!selected.global && <>
              <Button size="xs" variant="outline" disabled={busy || !dirty || !!validation} onClick={async () => { if (await mutate('POST', { Name: selected.key, YAML: yaml })) select({ ...selected, yaml }) }}><Save />Save</Button>
              <Button size="xs" variant="ghost" aria-label="Delete workflow" disabled={busy} onClick={() => { if (confirm('Delete workflow?')) void mutate('DELETE', { Name: selected.key }) }}><Trash2 /></Button>
            </>}
          </div>
          {(validation || error) && <p role="alert" className="shrink-0 whitespace-pre-wrap border-b bg-destructive/5 px-3 py-1.5 text-xs text-destructive">{validation && validationLine > 0 ? <button type="button" className="text-left underline decoration-dotted underline-offset-2" onClick={() => setGoto({ line: validationLine })}>Line {validationLine}: {validation}</button> : validation || error}</p>}
          <div className="grid min-h-0 flex-1 grid-cols-1 grid-rows-2 md:grid-cols-2 md:grid-rows-1">
            <div className="flex min-h-0 flex-col border-b md:border-b-0 md:border-r">
              <div className="shrink-0 px-3 pt-2 text-[11px] font-medium text-muted-foreground">YAML{selected.global && ' · global, read-only'}</div>
              {!selected.global && <div className="flex shrink-0 flex-wrap items-center gap-1 px-3 pt-1 text-[11px]" aria-label="Insert into YAML">
                <span className="text-muted-foreground">Insert</span>
                {[...['${{ next }}', '${{ status }}', '${{ ticket.id }}', '${{ ticket.title }}'], ...Object.keys(selected.inputs ?? {}).map(k => '$' + '{{ inputs.' + k + ' }}'), ...new Set((preview?.Jobs ?? selected.jobs ?? []).map(j => j.Status))]
                  .map(text => <button key={text} type="button" className="rounded border px-1.5 py-0.5 font-mono hover:bg-accent" onClick={() => setInsert({ text, n: (insert?.n ?? 0) + 1 })}>{text}</button>)}
              </div>}
              <div className={`min-h-0 flex-1 ${selected.global ? 'bg-muted/40' : ''}`}>
                <Suspense fallback={<pre className="h-full overflow-auto p-3 font-mono text-[12px] leading-5">{yaml}</pre>}>
                  <WorkflowYamlEditor key={selected.key + selected.global} value={yaml} onChange={setYAML} readOnly={selected.global} errorLine={validationLine} errorMessage={validation} goto={goto} insert={insert} onCursor={line => setActiveJob(jobAt(line))} />
                </Suspense>
              </div>
            </div>
            <div className="min-h-0 overflow-auto p-3">
              <div className="mb-2 text-[11px] font-medium text-muted-foreground">Flow{validation && ' · last valid version'}</div>
              <FlowChart jobs={preview?.Jobs ?? selected.jobs ?? []} active={activeJob} onSelect={selectJob} />
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
