import { Suspense, lazy, useEffect, useRef, useState } from 'react'
import { FileCode2, MoreHorizontal, RotateCcw, Save, Undo2 } from 'lucide-react'
import { cn } from '@/lib/utils'
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

// Section is one titled block of the overview: a small caps heading, an optional action, then its body.
function Section({ id, title, action, children }: { id: string; title: string; action?: React.ReactNode; children: React.ReactNode }) {
  return <section aria-labelledby={id} className="space-y-2">
    <div className="flex items-baseline gap-2">
      <h3 id={id} className="flex-1 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">{title}</h3>
      {action}
    </div>
    {children}
  </section>
}

const empty = (text: string) => <p className="rounded-lg border border-dashed px-3 py-4 text-center text-[12px] text-muted-foreground">{text}</p>

// BoardSummaryView is the readable side of the page: agents, triggers and the
// dispatch limit. It follows the draft live.
export function BoardSummaryView({ summary }: { summary?: BoardSummary }) {
  if (!summary) return empty('No valid configuration to preview.')
  return <>
    {/* Harness and model per agent are edited on the Agents page; here only a link, not a second copy. */}
    <a href="#/agents" className="flex items-center gap-2 rounded-lg border bg-background px-3 py-2.5 text-[12px] hover:bg-accent">
      <span className="font-medium">Agents</span>
      <span className="truncate text-muted-foreground">{summary.roles.map(r => r.name).join(', ') || 'none yet'}</span>
      <span className="ml-auto shrink-0 text-muted-foreground">Manage →</span>
    </a>
    <Section id="board-config-triggers" title="Triggers">
      {summary.triggers.length ? <ul className="divide-y rounded-lg border bg-background">
        {summary.triggers.map((t, i) => <li key={i}>
          <details className="group px-3 py-2 text-[12px]">
            <summary className="flex cursor-pointer list-none items-center gap-2">
              <span className="rounded bg-brand/10 px-1.5 py-px font-mono text-[11px] font-medium text-brand">{t.on === 'mention' ? '@mention' : t.on}</span>
              <span className="truncate text-muted-foreground">→ {t.agent === '{{event.name}}' ? 'the mentioned agent' : t.agent}{t.when && t.when !== 'to=*' ? ` · ${t.when}` : ''}</span>
              {t.prompt && <span className="ml-auto text-muted-foreground transition-transform group-open:rotate-90" aria-hidden>▸</span>}
            </summary>
            {t.prompt && <pre className="mt-2 overflow-x-auto rounded-md bg-muted/60 p-2 font-mono text-[11px] whitespace-pre-wrap text-muted-foreground">{t.prompt}</pre>}
          </details>
        </li>)}
      </ul> : empty('No triggers.')}
    </Section>
    <Section id="board-config-limit" title="Dispatch limit">
      <div className="flex items-center gap-3 rounded-lg border bg-background px-3 py-2.5">
        <p className="flex-1 text-[12px] text-muted-foreground">Most agent runs one ticket may start in an hour.</p>
        <span className="text-[13px] font-semibold tabular-nums">{summary.maxFiresPerHour}<span className="font-normal text-muted-foreground"> / hour</span></span>
      </div>
    </Section>
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
  const [view, setView] = useState<'overview' | 'yaml'>('overview')

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
    <header className="flex h-11 shrink-0 items-center gap-2 border-b px-3">
      <h2 className="text-[13px] font-semibold">Board settings</h2>
      {dirty && <span className="rounded-full bg-amber-500/15 px-2 py-px text-[11px] text-amber-600 dark:text-amber-400">Unsaved</span>}
      {/* Narrow screens show one side at a time; wide ones show both. */}
      <div role="tablist" aria-label="View" className="ml-2 flex rounded-md bg-muted p-0.5 text-[12px] lg:hidden">
        {(['overview', 'yaml'] as const).map(v => <button key={v} role="tab" aria-selected={view === v} type="button" onClick={() => setView(v)}
          className={cn('rounded px-2.5 py-0.5', view === v ? 'bg-background font-medium shadow-sm' : 'text-muted-foreground')}>{v === 'yaml' ? 'YAML' : 'Overview'}</button>)}
      </div>
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
    {(validation || error) && <p role="alert" className="shrink-0 border-b bg-destructive/5 px-3 py-1.5 text-xs whitespace-pre-wrap text-destructive">
      {validation && validationLine > 0
        ? <button type="button" className="text-left underline decoration-dotted underline-offset-2" onClick={() => { setView('yaml'); setGoto({ line: validationLine }) }}>Line {validationLine}: {validationMessage}</button>
        : validation || error}
    </p>}
    <div className="flex min-h-0 flex-1">
      <div className={cn('min-h-0 flex-1 space-y-5 overflow-auto p-4 lg:max-w-[420px] lg:flex-none lg:basis-[42%] lg:border-r', view !== 'overview' && 'max-lg:hidden')}>
        {/* Not a Section: the select itself carries the "Default workflow" label. */}
        <div className="space-y-2">
          <h3 id="board-config-workflow" className="text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">Default workflow</h3>
          <div className="rounded-lg border bg-background px-3 py-2.5">
            <select aria-label="Default workflow" disabled={busy || !ready} value={summary?.defaultWorkflow ?? ''} onChange={e => void chooseDefault(e.target.value)} className="h-8 w-full rounded-md border bg-card px-2 text-[13px] font-medium">
              <option value="">None</option>
              {workflows.map(w => <option key={w.name} value={w.name}>{w.name}{w.global ? ' (global)' : ''}</option>)}
            </select>
            <p className="mt-1.5 text-[11.5px] text-muted-foreground">New tickets run this flow. Flows live in .tasks/workflows/*.yml.</p>
          </div>
        </div>
        <BoardSummaryView summary={summary} />
      </div>
      <div className={cn('flex min-h-0 min-w-0 flex-1 flex-col', view !== 'yaml' && 'max-lg:hidden')}>
        <div className="flex h-9 shrink-0 items-center gap-2 border-b bg-muted/40 px-3">
          <FileCode2 className="size-3.5 text-muted-foreground" />
          <span className="truncate font-mono text-[12px]" title={`${dir}/.tasks/board.yaml`}>.tasks/board.yaml</span>
          {!exists && <span className="text-[11px] text-muted-foreground">not created yet</span>}
          <span className="ml-auto hidden text-[11px] text-muted-foreground sm:inline">Comments and key order are kept</span>
        </div>
        <div className="min-h-0 flex-1 bg-muted/20">
          <Suspense fallback={<pre className="h-full overflow-auto p-3 font-mono text-[12px] leading-5">{draft}</pre>}>
            <WorkflowYamlEditor key={dir} value={draft} onChange={setDraft} readOnly={false} errorLine={validationLine} errorMessage={validation} goto={goto} label="Board config YAML" wrap={false} />
          </Suspense>
        </div>
      </div>
    </div>
    <p className="shrink-0 border-t px-3 py-1.5 text-[11px] text-muted-foreground">Commands in workflows run on this machine. Invalid YAML is never saved.</p>
  </section>
}
