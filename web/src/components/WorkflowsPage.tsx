import { useState } from 'react'
import { api, send } from '@/api'
import { usePoll } from '@/hooks/usePoll'
import { enc } from '@/hooks/useRoute'
import { Button } from '@/components/ui/button'

type Flow = { key: string; name: string; global: boolean; yaml: string; description: string; error: string; jobs: { ID: string }[] }
export function WorkflowsPage({ dir }: { dir: string }) {
 const path = '/api/workflows?dir=' + enc(dir)
 const poll = usePoll(() => api<Flow[]>(path), 2000, dir)
 const [selected, select] = useState<Flow>()
 const [yaml, setYAML] = useState('')
 const [error, setError] = useState('')
 const [busy, setBusy] = useState(false)
 async function mutate(method: string, body: object) {
  setBusy(true); setError('')
  try { await send(method, path, body); poll.reload(); if (method === 'DELETE') select(undefined) }
  catch (e) { setError((e as Error).message) }
  finally { setBusy(false) }
 }
 return <section className="h-full overflow-auto rounded-lg border bg-card p-3 space-y-3">
  <header className="flex gap-3 items-center"><h2>Workflows</h2><Button size="sm" disabled={busy} onClick={() => { const name = prompt('Workflow name'); if (name) void mutate('POST', { Name: name, Op: 'new' }) }}>New</Button></header>
  {(poll.data ?? []).map(f => <button key={f.key + f.global} className="block w-full text-left rounded border p-2 hover:bg-accent" onClick={() => { select(f); setYAML(f.yaml); setError('') }}><b>{f.name}</b> · {f.global ? 'global (read-only)' : 'project'}<p>{f.description}</p><p className="font-mono text-xs">{f.jobs?.map(j => j.ID).join(' → ')}</p>{f.error && <p className="text-destructive">{f.error}</p>}</button>)}
  {!poll.data?.length && <p>No workflows yet.</p>}
  {selected && <div className="space-y-2"><h3>{selected.name}</h3><textarea aria-label="Workflow YAML" readOnly={selected.global} value={yaml} onChange={e => setYAML(e.target.value)} className="w-full h-80 rounded border bg-background p-3 font-mono text-xs" />
   {!selected.global && <div className="flex gap-2"><Button disabled={busy} onClick={() => void mutate('POST', { Name: selected.key, YAML: yaml })}>Save</Button><Button variant="outline" disabled={busy} onClick={() => { if (confirm('Delete workflow?')) void mutate('DELETE', { Name: selected.key }) }}>Delete</Button><Button variant="outline" disabled={busy} onClick={() => { const goal = prompt('Input goal'); if (goal) void mutate('POST', { Name: selected.key, Op: 'run', Goal: goal }) }}>Run</Button></div>}
   {selected.global && <Button variant="outline" disabled={busy} onClick={() => { const goal = prompt('Input goal'); if (goal) void mutate('POST', { Name: selected.key, Global: true, Op: 'run', Goal: goal }) }}>Run</Button>}
  </div>}
  {(error || poll.error) && <p role="alert" className="text-destructive whitespace-pre-wrap">{error || poll.error?.message}</p>}
 </section>
}
