import { useEffect, useRef, useState } from 'react'
import { api, send } from '@/api'
import { usePoll } from '@/hooks/usePoll'
import { enc } from '@/hooks/useRoute'
import { Button } from '@/components/ui/button'

type Job = { ID: string; Status: string; Agent: string; Model: string; Prompt: string; Run: string; Outputs: string[]; OnFailure: string; MaxLoops: number }
type Input = { Description: string; Default: string; Required: boolean }
type Flow = { key: string; name: string; global: boolean; yaml: string; description: string; error: string; jobs: Job[]; inputs: Record<string, Input>; total: number; running: number; lastUsed: string }
type Preview = { Name: string; Description: string; Jobs: Job[] }
export function WorkflowsPage({ dir }: { dir: string }) {
 return <WorkflowEditor key={dir} dir={dir} />
}
function WorkflowEditor({ dir }: { dir: string }) {
 const path = '/api/workflows?dir=' + enc(dir)
 const poll = usePoll(() => api<Flow[]>(path), 2000, dir)
 const [selected, select] = useState<Flow>()
 const [yaml, setYAML] = useState('')
 const [preview, setPreview] = useState<Preview>()
 const [tab, setTab] = useState('form')
 const [validation, setValidation] = useState('')
 const [error, setError] = useState('')
 const [busy, setBusy] = useState(false)
 const [run, setRun] = useState<Flow>()
 const [values, setValues] = useState<Record<string, string>>({})
 const revision = useRef(0)
 useEffect(() => {
  if (!selected) return
  const id = ++revision.current
  let active = true
  const timer = setTimeout(() => {
   void send('POST', path, { Op: 'validate', YAML: yaml }).then(result => {
    if (!active || id !== revision.current) return
    setPreview((result as { preview: Preview }).preview); setValidation('')
   }).catch(e => { if (active && id === revision.current) { setValidation((e as Error).message); setPreview(undefined) } })
  }, 250)
  return () => { active = false; clearTimeout(timer) }
 }, [yaml, selected, path])
 async function mutate(method: string, body: object) {
  setBusy(true); setError('')
  try { await send(method, path, body); poll.reload(); if (method === 'DELETE') select(undefined); return true }
  catch (e) { setError((e as Error).message); return false }
  finally { setBusy(false) }
 }
 async function patch(Patch: object) {
  setBusy(true)
  try {
   const result = await send('POST', path, { Op: 'patch', YAML: yaml, Patch }) as { yaml: string; preview: Preview }
   setYAML(result.yaml); setPreview(result.preview); setValidation('')
  } catch (e) { setError((e as Error).message) } finally { setBusy(false) }
 }
 function open(f: Flow) {
  if (selected && yaml !== selected.yaml && !confirm('Discard unsaved workflow changes?')) return
  select(f); setYAML(f.yaml); setPreview({ Name: f.name, Description: f.description, Jobs: f.jobs }); setError(''); setValidation(f.error)
 }
 function start(f: Flow) { setRun(f); setError(''); setValues(Object.fromEntries(Object.entries(f.inputs ?? {}).map(([key, value]) => [key, value.Default]))) }
 const fieldClass = 'w-full rounded border bg-background p-2'
 return <section className="h-full overflow-auto rounded-lg border bg-card p-3 space-y-4">
  <header className="flex gap-3 items-center"><h2 className="text-lg font-semibold">Workflows</h2><Button size="sm" disabled={busy} onClick={() => { const name = prompt('Workflow filename (letters, numbers and hyphens)'); if (name) void mutate('POST', { Name: name, Op: 'new' }) }}>New</Button></header>
  <p className="text-muted-foreground">Choose a recipe to create a ticket and guide it through each step.</p>
  {poll.error ? <p role="alert">We couldn't load workflows. {poll.error.message} <Button onClick={poll.reload}>Try again</Button></p> : !poll.data ? <p>Loading workflows…</p> : !poll.data.length ? <p>No workflows yet. Choose New to create your first recipe.</p> : null}
  <div className="grid gap-3 md:grid-cols-2">{(poll.data ?? []).map(f => <article key={f.key + f.global} className="rounded-lg border p-4 space-y-2">
   <h3 className="font-semibold">{f.name} <span className="text-xs text-muted-foreground">{f.global ? 'Global · read-only' : 'Project'}</span></h3>
   <p>{f.description || 'A sequence of steps for a new ticket.'}</p>
   <p className="text-sm text-muted-foreground">{f.jobs?.length ?? 0} steps · {f.total} tickets · {f.running} in progress</p>
   <p className="text-xs" title="Latest associated ticket creation; same-name workflows share counts">{f.total ? `Last used: ${new Date(f.lastUsed).toLocaleString()}` : 'Never used'}</p>
   {f.error && <p role="alert" className="text-destructive">This recipe needs fixing: {f.error}</p>}
   <div className="flex gap-2"><Button disabled={busy || !!f.error} onClick={() => start(f)}>Run</Button><Button variant="outline" onClick={() => open(f)}>{f.global ? 'View' : 'Edit / explore'}</Button></div>
  </article>)}</div>
  {selected && <div className="space-y-3 border-t pt-4"><h3 className="font-semibold">{selected.name}</h3>
   <div className="flex flex-col md:flex-row gap-3 overflow-auto">{(preview?.Jobs ?? selected.jobs ?? []).map(j => <details key={j.ID} className="border-l-4 border-primary rounded border p-3 md:min-w-64 md:max-w-96 shrink-0">
    <summary className="cursor-pointer"><b>{j.Run ? '⚙' : '●'} {j.ID}</b><p className="text-sm">Status: {j.Status} · {j.Run ? 'CI command' : j.Agent}</p><p className="text-xs">{j.Model || 'Model: role default'}</p><p className="truncate text-sm">{j.Run || j.Prompt || 'Uses the role prompt'}</p></summary>
    <pre className="whitespace-pre-wrap break-words text-xs py-2">{j.Run || j.Prompt || 'Uses the role prompt'}</pre>
    <p className="text-xs">Outputs: {j.Outputs?.join(', ') || 'None declared'}</p><p className="text-xs">On failure → {j.OnFailure || 'blocked'} · Max loops: {j.MaxLoops || 'default'}; exhaustion → blocked</p>
   </details>)}</div>
   <p className="text-xs text-muted-foreground">Steps run in order → done. Expand a step for the full prompt or command. Counts include blocked tickets; live worker activity is available on the board.</p>
   <div className="flex gap-2"><Button variant="outline" onClick={() => setTab('form')}>Form</Button><Button variant="outline" onClick={() => setTab('yaml')}>YAML (advanced)</Button></div>
   {selected.global && <p>Global recipes are read-only. Create a project recipe to customize one.</p>}
   {tab === 'yaml' ? <textarea aria-label="Workflow YAML" readOnly={selected.global} value={yaml} onChange={e => { setYAML(e.target.value); setPreview(undefined) }} className={fieldClass + ' h-80 font-mono text-xs'} /> : preview && !validation ? <fieldset disabled={selected.global || busy} className="space-y-3">
    {(['name', 'description'] as const).map(key => <label key={key} className="block">{key}<input key={yaml + key} defaultValue={key === 'name' ? preview.Name : preview.Description} className={fieldClass} onBlur={e => { const old = key === 'name' ? preview.Name : preview.Description; if (e.target.value !== old) void patch({ Fields: { [key]: e.target.value } }) }} /></label>)}
    {preview.Jobs.map((j, i) => <div key={j.ID} className="rounded border p-3 space-y-2"><h4>Step {i + 1}: {j.ID}</h4>
     {(['agent', 'model', 'prompt', 'run', 'on-failure', 'status'] as const).map(key => { const prop = { agent: 'Agent', model: 'Model', prompt: 'Prompt', run: 'Run', 'on-failure': 'OnFailure', status: 'Status' }[key] as keyof Job; return <label key={key} className="block text-sm">{key}<textarea key={yaml + key} defaultValue={String(j[prop] ?? '')} className={fieldClass} onBlur={e => { if (e.target.value !== String(j[prop] ?? '')) void patch({ Jobs: { [j.ID]: { [key]: e.target.value } } }) }} /></label> })}
     <p className="text-xs">Choose either a role (agent) or a command (run), not both. Use YAML to switch modes together.</p>
     <div className="flex gap-2"><Button variant="outline" disabled={i === 0 || busy} onClick={() => { const ids = preview.Jobs.map(j => j.ID); [ids[i - 1], ids[i]] = [ids[i], ids[i - 1]]; void patch({ Steps: ids }) }}>Move up</Button><Button variant="outline" onClick={() => void patch({ Steps: preview.Jobs.filter(x => x.ID !== j.ID).map(x => x.ID) })}>Remove</Button></div>
    </div>)}
    <Button variant="outline" onClick={() => { const id = prompt('New step ID'); if (id) void patch({ Steps: [...preview.Jobs.map(j => j.ID), id] }) }}>Add step</Button>
   </fieldset> : <p>Fix the YAML errors before using the form.</p>}
   {validation && <p role="alert" className="text-destructive whitespace-pre-wrap">{validation}</p>}
   {!selected.global && <div className="flex gap-2"><Button disabled={busy || !!validation || !preview} onClick={async () => { if (await mutate('POST', { Name: selected.key, YAML: yaml })) select({ ...selected, yaml }) }}>Save</Button><Button variant="outline" disabled={busy} onClick={() => { if (confirm('Delete workflow?')) void mutate('DELETE', { Name: selected.key }) }}>Delete</Button></div>}
  </div>}
  {run && <div role="dialog" aria-modal="true" aria-label={'Run ' + run.name} className="fixed inset-0 z-50 bg-background/95 overflow-auto p-6"><form className="mx-auto max-w-lg space-y-4" onSubmit={async e => { e.preventDefault(); if (await mutate('POST', { Name: run.key, Global: run.global, Op: 'run', Inputs: values })) setRun(undefined) }}>
   <h3>Run {run.name}</h3><p>This creates a new ticket and starts its first step.</p>
   {Object.entries(run.inputs ?? {}).map(([key, input]) => <label key={key} className="block">{key} {input.Required ? '(required)' : '(optional)'}<p className="text-sm text-muted-foreground">{input.Description}</p><textarea autoFocus required={input.Required} className={fieldClass} value={values[key] ?? ''} onChange={e => setValues({ ...values, [key]: e.target.value })} /></label>)}
   {!Object.keys(run.inputs ?? {}).length && <p>No inputs needed.</p>}
   {error && <p role="alert" className="text-destructive">{error}</p>}<Button type="submit" disabled={busy}>Run</Button> <Button type="button" variant="outline" disabled={busy} onClick={() => setRun(undefined)}>Cancel</Button>
  </form></div>}
  {error && !run && <p role="alert" className="text-destructive whitespace-pre-wrap">{error}</p>}
 </section>
}
