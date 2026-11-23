import { Suspense, lazy, useEffect, useRef, useState } from 'react'
import { MoreHorizontal, RotateCcw, Save, Undo2 } from 'lucide-react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
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

// One settings row: label and a short hint on the left, the value or control on the right.
function Row({ label, hint, children, id }: { label: string; hint?: string; children?: React.ReactNode; id?: string }) {
  return <div className="flex items-start gap-4 border-b px-4 py-3 last:border-b-0">
    <div className="min-w-0 flex-1">
      <h3 id={id} className="text-[13px] font-medium">{label}</h3>
      {hint && <p className="mt-0.5 text-[11.5px] text-muted-foreground">{hint}</p>}
    </div>
    {children && <div className="shrink-0">{children}</div>}
  </div>
}

const shortModel = (m?: string) => (m ? m.slice(m.lastIndexOf('/') + 1) : 'default')

// BoardSummaryView is the readable side of the page: agents and triggers, as
// one list of settings rows. It follows the draft live.
export function BoardSummaryView({ summary }: { summary?: BoardSummary }) {
  if (!summary) return <p className="px-4 py-3 text-sm text-muted-foreground">No valid configuration to preview.</p>
  return <>
    <section aria-labelledby="board-config-roles" className="border-b">
      <div className="flex items-baseline gap-2 px-4 pt-3">
        <h3 id="board-config-roles" className="flex-1 text-[13px] font-medium">Agents</h3>
        <a href="#/agents" className="text-[11.5px] text-muted-foreground hover:text-foreground hover:underline">Manage agents →</a>
      </div>
      {summary.roles.length ? <table className="mx-4 mb-3 mt-1.5 w-[calc(100%-2rem)] text-[12px]">
        <tbody>{summary.roles.map(r => <tr key={r.name} className="border-t first:border-t-0">
          <td className="py-1.5 pr-3 font-medium">{r.name}</td>
          <td className="py-1.5 pr-3 text-muted-foreground">{r.agent}</td>
          <td className="w-full truncate py-1.5 font-mono text-[11.5px] text-muted-foreground" title={[r.agent, r.model].filter(Boolean).join(' ')}>
            {shortModel(r.model)}{r.effort ? ` · effort ${r.effort}` : ''}{r.freshAfter != null ? ` · fresh after ${r.freshAfter}` : ''}
          </td>
        </tr>)}</tbody>
      </table> : <p className="px-4 pb-3 pt-1 text-[11.5px] text-muted-foreground">No agents yet.</p>}
    </section>
    <section aria-labelledby="board-config-triggers" className="border-b">
      <h3 id="board-config-triggers" className="px-4 pt-3 text-[13px] font-medium">Triggers</h3>
      {summary.triggers.length ? <ul className="px-4 pb-2 pt-1">
        {summary.triggers.map((t, i) => <li key={i}>
          <details className="group border-t py-1.5 text-[12px] first:border-t-0">
            <summary className="flex cursor-pointer list-none items-center gap-2">
              <span className="font-medium">{t.on === 'mention' ? '@mention' : t.on}</span>
              <span className="truncate text-muted-foreground">→ {t.agent === '{{event.name}}' ? 'the mentioned agent' : t.agent}{t.when && t.when !== 'to=*' ? ` · ${t.when}` : ''}</span>
              <span className="ml-auto text-muted-foreground transition-transform group-open:rotate-90" aria-hidden>▸</span>
            </summary>
            {t.prompt && <p className="mt-1 whitespace-pre-wrap font-mono text-[11px] text-muted-foreground">{t.prompt}</p>}
          </details>
        </li>)}
      </ul> : <p className="px-4 pb-3 pt-1 text-[11.5px] text-muted-foreground">No triggers.</p>}
    </section>
    <Row label="Dispatch limit" hint="Most agent runs one ticket may start in an hour.">
      <span className="text-[12px] tabular-nums">{summary.maxFiresPerHour} / hour</span>
    </Row>
  </>
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
      <h2 className="text-[13px] font-semibold">Board</h2>
      <span className="truncate font-mono text-[11px] text-muted-foreground" title={`${dir}/.tasks/board.yaml`}>.tasks/board.yaml{!exists && ' · not created yet'}</span>
      {dirty && <span className="text-[11px] text-amber-600">● Unsaved</span>}
      <div className="ml-auto flex items-center gap-1">
        <Button size="xs" variant="ghost" disabled={busy || !dirty} onClick={() => setDraft(saved)}><Undo2 />Revert</Button>
        <Button size="xs" disabled={busy || !dirty || invalid} onClick={() => void save()}><Save />Save</Button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild><Button size="icon-xs" variant="ghost" aria-label="More actions"><MoreHorizontal /></Button></DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem disabled={busy || !defaults} onSelect={() => setDraft(defaults)}><RotateCcw />Restore defaults</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
    {(validation || error) && <p role="alert" className="shrink-0 whitespace-pre-wrap border-b bg-destructive/5 px-3 py-1.5 text-xs text-destructive">
      {validation && validationLine > 0
        ? <button type="button" className="text-left underline decoration-dotted underline-offset-2" onClick={() => setGoto({ line: validationLine })}>Line {validationLine}: {validationMessage}</button>
        : validation || error}
    </p>}
    <div className="grid min-h-0 flex-1 grid-rows-2 md:grid-cols-2 md:grid-rows-1">
      <div className="min-h-0 overflow-auto border-b md:border-b-0 md:border-r">
        <Row id="board-config-workflow" label="Default workflow" hint="New tickets run this flow. Flows live in .tasks/workflows/*.yml.">
          <select aria-label="Default workflow" disabled={busy || !ready} value={summary?.defaultWorkflow ?? ''} onChange={e => void chooseDefault(e.target.value)} className="h-7 rounded-md border bg-background px-2 text-[12px]">
            <option value="">None</option>
            {workflows.map(w => <option key={w.name} value={w.name}>{w.name}{w.global ? ' (global)' : ''}</option>)}
          </select>
        </Row>
        <BoardSummaryView summary={summary} />
      </div>
      <div className="flex min-h-0 flex-col">
        <div className="shrink-0 border-b px-3 py-1.5 text-[11px] text-muted-foreground">YAML · comments and order are kept</div>
        <div className="min-h-0 flex-1">
          <Suspense fallback={<pre className="h-full overflow-auto p-3 font-mono text-[12px] leading-5">{draft}</pre>}>
            <WorkflowYamlEditor key={dir} value={draft} onChange={setDraft} readOnly={false} errorLine={validationLine} errorMessage={validation} goto={goto} label="Board config YAML" />
          </Suspense>
        </div>
      </div>
    </div>
    <p className="shrink-0 border-t px-3 py-1.5 text-[11px] text-muted-foreground">Commands in workflows run on this machine. Invalid YAML is never saved.</p>
  </section>
}
