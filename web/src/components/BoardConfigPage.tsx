import { Suspense, lazy, useEffect, useRef, useState } from 'react'
import { Bot, RotateCcw, Save, Terminal, Undo2 } from 'lucide-react'
import { api, send } from '@/api'
import { Button } from '@/components/ui/button'
import { enc } from '@/hooks/useRoute'

// The editor is CodeMirror: lazy-loaded so it stays out of the main bundle, like the workflows page.
const WorkflowYamlEditor = lazy(() => import('@/components/WorkflowYamlEditor'))

export type BoardRoleSummary = { name: string; agent: string; model?: string; effort?: string; freshAfter?: number }
export type BoardTriggerSummary = { on: string; when?: string; agent: string; prompt?: string }
export type BoardSummary = { defaultWorkflow?: string; roles: BoardRoleSummary[]; triggers: BoardTriggerSummary[]; maxFiresPerHour: number }
export type BoardWorkflowOption = { name: string; global: boolean }

type Config = { dir: string; path: string; yaml: string; default: string; exists: boolean; error?: string; line?: number; summary?: BoardSummary; workflows?: BoardWorkflowOption[] }

// parseError reads the server's JSON {error, line}; plain-text failures have no line.
function parseError(text: string): { error: string; line: number } {
  try {
    const v = JSON.parse(text) as { error?: unknown; line?: unknown }
    if (typeof v.error === 'string') return { error: v.error, line: Number(v.line) || 0 }
  } catch { /* plain text */ }
  return { error: text, line: 0 }
}

// BoardSummaryView is the readable side of the page: the default workflow,
// roles with harness and model, triggers. It follows the draft live.
export function BoardSummaryView({ summary }: { summary?: BoardSummary }) {
  if (!summary) return <p className="text-sm text-muted-foreground">No valid configuration to preview.</p>
  return <div className="space-y-5">
    <section aria-labelledby="board-config-workflow">
      <h3 id="board-config-workflow" className="mb-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">Default workflow</h3>
      <p className="rounded-md border bg-card px-2.5 py-1.5 text-[12.5px] font-semibold">{summary.defaultWorkflow || 'None — pick a workflow when starting a ticket'}</p>
      <p className="mt-1 text-[11px] text-muted-foreground">Flows live in .tasks/workflows/*.yml. Lanes are fixed: todo, in progress, blocked, done.</p>
    </section>
    <section aria-labelledby="board-config-roles">
      <h3 id="board-config-roles" className="mb-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">Roles</h3>
      {summary.roles.length ? <ul className="space-y-1">
        {summary.roles.map(r => <li key={r.name} className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1.5 text-[12px]">
          <Bot className="size-3.5 shrink-0 text-indigo-600" aria-hidden />
          <b>{r.name}</b>
          <span className="truncate text-[11.5px] text-muted-foreground">{[r.agent, r.model].filter(Boolean).join(' ')}{r.effort ? ` · effort ${r.effort}` : ''}{r.freshAfter != null ? ` · fresh after ${r.freshAfter}` : ''}</span>
        </li>)}
      </ul> : <p className="text-[11.5px] text-muted-foreground">No roles configured.</p>}
    </section>
    <section aria-labelledby="board-config-triggers">
      <h3 id="board-config-triggers" className="mb-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">Triggers</h3>
      {summary.triggers.length ? <ul className="space-y-1">
        {summary.triggers.map((t, i) => <li key={i} className="rounded-md border bg-card px-2.5 py-1.5 text-[12px]">
          <div className="flex items-center gap-2"><Terminal className="size-3.5 shrink-0 text-sky-600" aria-hidden /><b>{t.on}</b>{t.when && <span className="truncate text-[11.5px] text-muted-foreground">{t.when}</span>}</div>
          <p className="pl-5 text-[11px] text-muted-foreground">agent {t.agent}</p>
        </li>)}
      </ul> : <p className="text-[11.5px] text-muted-foreground">No triggers configured.</p>}
    </section>
    <p className="text-[11px] text-muted-foreground">max_fires_per_hour: {summary.maxFiresPerHour}</p>
  </div>
}

export function BoardConfigPage({ dir }: { dir: string }) {
  const path = '/api/board/config?dir=' + enc(dir)
  const [draft, setDraft] = useState('')
  const [saved, setSaved] = useState('')
  const [defaults, setDefaults] = useState('')
  const [exists, setExists] = useState(false)
  const [ready, setReady] = useState(false)
  const [summary, setSummary] = useState<BoardSummary>()
  const [workflows, setWorkflows] = useState<BoardWorkflowOption[]>([])
  const [validation, setValidation] = useState('')
  const [validationLine, setValidationLine] = useState(0)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [goto, setGoto] = useState<{ line: number; to?: number }>()
  const revision = useRef(0)

  useEffect(() => {
    let active = true
    api<Config>(path).then(c => {
      if (!active) return
      setDraft(c.yaml); setSaved(c.yaml); setDefaults(c.default); setExists(c.exists)
      setSummary(c.summary); setWorkflows(c.workflows ?? []); setValidation(c.error ?? ''); setValidationLine(c.line ?? 0)
      setReady(true)
    }).catch(e => { if (active) { setError((e as Error).message); setReady(true) } })
    return () => { active = false }
  }, [path])

  // Re-validate the draft as it is typed so the summary follows the text and
  // Save is refused before the server writes anything.
  useEffect(() => {
    if (!ready) return
    const id = ++revision.current
    let active = true
    const timer = setTimeout(() => {
      void send('PUT', path, { yaml: draft, validate: true }).then(result => {
        if (!active || id !== revision.current) return
        setSummary((result as { summary: BoardSummary }).summary); setValidation(''); setValidationLine(0)
      }).catch(e => {
        if (!active || id !== revision.current) return
        const { error, line } = parseError((e as Error).message)
        setValidation(error); setValidationLine(line)
      })
    }, 250)
    return () => { active = false; clearTimeout(timer) }
  }, [draft, ready, path])

  // The select edits only default_workflow through the server, so comments and
  // key order in the YAML half survive. The editor reloads from the response.
  async function chooseDefault(name: string) {
    setBusy(true); setError('')
    try {
      const c = await send<Config>('PUT', '/api/board/config/default-workflow?dir=' + enc(dir), { name })
      setDraft(c.yaml); setSaved(c.yaml); setExists(true); setSummary(c.summary); setWorkflows(c.workflows ?? workflows)
      setValidation(c.error ?? ''); setValidationLine(c.line ?? 0)
    } catch (e) {
      const { error, line } = parseError((e as Error).message)
      setError(error); setValidation(error); setValidationLine(line)
    } finally { setBusy(false) }
  }

  const dirty = draft !== saved
  const invalid = !!validation
  // The server's message already starts with "line N: "; the button adds it once.
  const validationMessage = validation.replace(new RegExp(`^line ${validationLine}: `), '')
  async function save() {
    setBusy(true); setError('')
    try {
      const result = await send<Config>('PUT', path, { yaml: draft })
      setSaved(draft); setExists(true); setSummary(result.summary); setValidation(''); setValidationLine(0)
    } catch (e) {
      const { error, line } = parseError((e as Error).message)
      setError(error); setValidation(error); setValidationLine(line)
    } finally { setBusy(false) }
  }

  return <section aria-label="Board config" className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border bg-card">
    <header className="flex h-10 shrink-0 items-center gap-2 border-b px-3">
      <h2 className="text-[13px] font-semibold">Board config</h2>
      <span className="truncate text-[11px] text-muted-foreground">{dir}/.tasks/board.yaml{!exists && ' · not created yet'}</span>
      <div className="ml-auto flex items-center gap-1.5">
        <Button size="xs" variant="ghost" disabled={busy || !defaults} onClick={() => setDraft(defaults)}><RotateCcw />Restore defaults</Button>
        <Button size="xs" variant="ghost" disabled={busy || !dirty} onClick={() => setDraft(saved)}><Undo2 />Revert</Button>
        <Button size="xs" disabled={busy || !dirty || invalid} onClick={() => void save()}><Save />Save</Button>
      </div>
    </header>
    {(validation || error) && <p role="alert" className="shrink-0 whitespace-pre-wrap border-b bg-destructive/5 px-3 py-1.5 text-xs text-destructive">
      {validation && validationLine > 0
        ? <button type="button" className="text-left underline decoration-dotted underline-offset-2" onClick={() => setGoto({ line: validationLine })}>Line {validationLine}: {validationMessage}</button>
        : validation || error}
    </p>}
    <div className="grid min-h-0 flex-1 grid-rows-2 md:grid-cols-2 md:grid-rows-1">
      <div className="min-h-0 overflow-auto border-b p-3 md:border-b-0 md:border-r">
        <label className="mb-4 block text-[11px] font-medium uppercase tracking-wide text-muted-foreground">Default workflow
          <select aria-label="Default workflow" disabled={busy || !ready} value={summary?.defaultWorkflow ?? ''} onChange={e => void chooseDefault(e.target.value)} className="mt-1 w-full rounded-md border bg-background px-2 py-1 text-[12px] normal-case tracking-normal text-foreground">
            <option value="">None — pick when starting a ticket</option>
            {workflows.map(w => <option key={w.name} value={w.name}>{w.name}{w.global ? ' (global)' : ''}</option>)}
          </select>
        </label>
        <BoardSummaryView summary={summary} />
      </div>
      <div className="flex min-h-0 flex-col">
        <div className="shrink-0 px-3 pt-2 text-[11px] font-medium text-muted-foreground">YAML · saved to .tasks/board.yaml, comments and order kept</div>
        <div className="min-h-0 flex-1">
          <Suspense fallback={<pre className="h-full overflow-auto p-3 font-mono text-[12px] leading-5">{draft}</pre>}>
            <WorkflowYamlEditor key={dir} value={draft} onChange={setDraft} readOnly={false} errorLine={validationLine} errorMessage={validation} goto={goto} label="Board config YAML" />
          </Suspense>
        </div>
      </div>
    </div>
    <p className="shrink-0 border-t px-3 py-1.5 text-[11px] text-muted-foreground">run: commands execute locally on this machine. Saving validates first; a broken config is never written.</p>
  </section>
}
