import { Suspense, lazy, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowLeft, Play, Plus, Save, Trash2, Workflow as WorkflowIcon } from 'lucide-react'
import { api, send } from '@/api'
import { usePoll } from '@/hooks/usePoll'
import { enc } from '@/hooks/useRoute'
import { Button } from '@/components/ui/button'
import { statusColor } from '@/components/BoardDrawer'
import { jobRanges } from '@/components/workflowYaml'
import { PaneHeader } from '@/components/PaneChrome'
import { FlowCanvas } from '@/components/FlowCanvas'
import { type Job, jobsToGraph, layoutGraph } from '@/lib/graphLayout'

const WorkflowYamlEditor = lazy(() => import('@/components/WorkflowYamlEditor'))

const NO_JOBS: Job[] = []
type Input = { Description: string; Default: string; Required: boolean }
export type Flow = { key: string; name: string; global: boolean; yaml: string; description: string; error: string; errorLine: number; jobs: Job[]; inputs: Record<string, Input>; total: number; running: number; lastUsed: string; workplace: string; cleanup: string }
type Preview = { Name: string; Description: string; Jobs: Job[] }

// onDirty reports unsaved YAML, so a pane never resizes away a draft.
export function WorkflowsPage({ dir, initial = '', onDirty }: { dir: string; initial?: string; onDirty?: (dirty: boolean) => void }) {
  return <WorkflowEditor key={dir} dir={dir} initial={initial} onDirty={onDirty} />
}

// FlowDetails shows the selected step's prompt or command, outputs and limits. The canvas shows the shape.
function FlowDetails({ job }: { job?: Job }) {
  if (!job) return <p className="shrink-0 border-t pt-2 text-xs text-muted-foreground">Select a step to see its prompt or command.</p>
  return <div className="max-h-44 shrink-0 space-y-1 overflow-auto border-t pt-2 text-xs">
    <p className="font-medium">{job.ID}<span className="font-normal text-muted-foreground"> · {job.Status}{job.Run ? ' · command' : job.Agent ? ` · ${job.Agent}` : ''}{job.Model ? ` · ${job.Model}` : ''}{job.MaxLoops > 0 ? ` · max ${job.MaxLoops}` : ''}</span></p>
    {!!job.Needs?.length && <p className="text-muted-foreground">Needs: {job.Needs.join(', ')}</p>}
    <pre className="max-h-32 overflow-auto whitespace-pre-wrap break-words font-mono">{job.Run || job.Prompt || 'Uses the role prompt'}</pre>
    {!!job.Outputs?.length && <p className="text-muted-foreground">Outputs: {job.Outputs.join(', ')}</p>}
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

function WorkflowEditor({ dir, initial, onDirty }: { dir: string; initial: string; onDirty?: (dirty: boolean) => void }) {
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
  const jobs = preview?.Jobs ?? selected?.jobs ?? NO_JOBS
  const graph = useMemo(() => { const g = jobsToGraph(jobs); return layoutGraph(g.nodes, g.edges) }, [jobs])
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
  useEffect(() => { onDirty?.(dirty); return () => onDirty?.(false) }, [dirty, onDirty])
  function open(f?: Flow) {
    if (dirty && !confirm('Discard unsaved workflow changes?')) return
    select(f); setYAML(f?.yaml ?? ''); setPreview(f && { Name: f.name, Description: f.description, Jobs: f.jobs }); setError(''); setValidation(f?.error ?? ''); setValidationLine(f?.errorLine ?? 0); setActiveJob(''); setGoto(undefined)
  }
  function start(f: Flow) { setRun(f); setError(''); setValues(Object.fromEntries(Object.entries(f.inputs ?? {}).map(([key, value]) => [key, value.Default]))) }
  const flows = poll.data ?? []
  // The saved row, not the snapshot: Save refreshes the workplace, and the draft is not what runs.
  const live = flows.find(f => f.key === selected?.key && f.global === selected?.global)
  // Opened with a name (#/workflows/<name>): select it once the list arrives.
  const [pending, setPending] = useState(initial)
  if (pending && poll.data) { setPending(''); const f = flows.find(x => x.name === pending || x.key === pending); if (f) open(f) }
  const fieldClass = 'w-full rounded border bg-background p-2'
  // Flow card → YAML: jump to the job's lines. YAML cursor → flow: the job under the cursor rings its card.
  function jobAt(line: number) { return Object.entries(jobRanges(yaml)).find(([, r]) => line >= r.from && line <= r.to)?.[0] ?? '' }
  function selectJob(id: string) { const r = jobRanges(yaml)[id]; setActiveJob(id); if (r) setGoto({ line: r.from, to: r.to }) }

  function newFlow() { const name = prompt('Workflow filename (letters, numbers and hyphens)'); if (name) void mutate('POST', { Name: name, Op: 'new' }) }
  return <section className="flex h-full min-h-0 select-text flex-col overflow-hidden rounded-lg border bg-card">
    <PaneHeader className="h-10 shrink-0 gap-2 border-b px-3">
      {selected && <Button variant="ghost" size="icon-xs" className="md:hidden" aria-label="Back to workflows" onClick={() => open(undefined)}><ArrowLeft /></Button>}
      <WorkflowIcon className="size-4 text-teal-600" /><h2 className="text-[13px] font-semibold">Workflows</h2>
      <Button size="xs" variant="outline" className="ml-auto" disabled={busy} onClick={newFlow}><Plus />New</Button>
    </PaneHeader>
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
            <h3 className="mr-auto truncate text-[13px] font-semibold">{selected.name}{dirty && <span className="ml-1 text-amber-600">•</span>}<span className="ml-2 text-[11px] font-normal text-muted-foreground">{selected.total} tickets · {selected.running} in progress</span>{live?.workplace && <span className="ml-2 text-[11px] font-normal text-muted-foreground">workplace {live.workplace}{live.cleanup && ` · cleanup ${live.cleanup}`}</span>}</h3>
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
            <div className="flex min-h-0 flex-col p-3">
              <div className="mb-2 shrink-0 text-[11px] font-medium text-muted-foreground">Flow{validation && ' · last valid version'}</div>
              <div className="flex min-h-0 flex-1 flex-col">
                <FlowCanvas key={selected.key + selected.global} graph={graph} active={activeJob} onSelect={selectJob} />
                <FlowDetails job={jobs.find(j => j.ID === activeJob)} />
              </div>
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
