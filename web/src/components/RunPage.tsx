import { useEffect, useState, type ReactNode } from "react"
import { ArrowLeft, CircleDot, Loader2, Pencil, Repeat, Terminal } from "lucide-react"
import { api } from "@/api"
import { usePoll } from "@/hooks/usePoll"
import { enc, nav } from "@/hooks/useRoute"
import { useAppState } from "@/state"
import { AttachmentRows, AttachmentViewer, type Attachment } from "@/components/Attachments"
import { Avatar, Md, shortId, statusColor } from "@/components/BoardDrawer"
import { attachmentURL } from "@/lib/attachments"
import { entryLabel, findAttachment, formatDuration, resultChip } from "@/lib/run"
import { Button } from "@/components/ui/button"
import { Panel } from "@/components/Panel"
import { baseName } from "@/lib/status"
import type { RunEntry, RunView as RunData } from "@/types"

// RunView shows one ticket's run: every entry into every step, with what was
// sent, what came back and how long it took. No YAML, no workflow list.
// page=false embeds it in the ticket drawer's Run tab; page=true is #/runs/<ticket>.
export function RunView({ dir, id, page = false }: { dir: string; id: string; page?: boolean }) {
  const poll = usePoll(() => api<RunData>(`/api/tasks/${enc(id)}/run?dir=${enc(dir)}`), 2000, dir + id)
  const run = poll.data
  const entries = run?.entries ?? []
  const { state } = useAppState()
  const live = new Set((state?.agents ?? []).map(a => a.pane))
  const [attachments, setAttachments] = useState<Attachment[]>([])
  const [selected, setSelected] = useState<Attachment | null>(null)
  const url = attachmentURL(dir, id)
  const count = entries.length
  useEffect(() => {
    const controller = new AbortController()
    fetch(url, { signal: controller.signal }).then(r => (r.ok ? r.json() : [])).then(setAttachments).catch(() => {})
    return () => controller.abort()
  }, [url, count])
  const clock = (iso?: string) => (iso ? new Date(iso).toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }) : "")
  const attachment = (name: string, commentId?: string) => findAttachment(attachments, name, commentId)

  if (poll.error) return page
    ? <Panel title="Run" className="m-2 self-start"><div className="space-y-2 p-3 text-sm"><p role="alert">Couldn't load the run in <b>{baseName(dir)}</b>. {poll.error.message}</p><Button variant="outline" size="sm" onClick={() => nav("/")}>Back to board</Button></div></Panel>
    : <p role="alert" className="p-4 text-sm">Couldn't load the run. {poll.error.message}</p>
  if (!run) return <p className="p-4 text-sm text-muted-foreground">Loading…</p>

  const openButton = (a: Attachment, label: string) => (
    <button type="button" onClick={() => setSelected(a)} className="inline-flex items-center gap-1 rounded border bg-muted/40 px-1.5 py-0.5 font-mono text-[11px] hover:border-foreground/30 hover:bg-accent">
      <Terminal className="size-3" />{label}
    </button>
  )

  const list = <div className="mx-auto w-full max-w-3xl space-y-3">
    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-[11.5px] text-muted-foreground">
      <span className="rounded px-1.5 py-px font-medium" style={{ color: statusColor(run.status), background: "var(--muted)" }}>{run.status}</span>
      <span>{entries.length} {entries.length === 1 ? "entry" : "entries"}</span>
      {run.started && <span>started {clock(run.started)}</span>}
      <span>total {formatDuration(run.total)}</span>
    </div>
    {!!Object.keys(run.inputs ?? {}).length && <dl className="flex flex-wrap gap-x-3 gap-y-1 rounded-md border bg-muted/30 px-2 py-1.5 text-[11.5px]">
      {Object.entries(run.inputs ?? {}).map(([k, v]) => <div key={k} className="flex min-w-0 gap-1"><dt className="shrink-0 text-muted-foreground">{k}:</dt><dd className="truncate font-medium" title={v}>{v}</dd></div>)}
    </dl>}
    <ol aria-label="Flow" className="flex gap-1 overflow-x-auto pb-1">
      {run.flow.map(s => <li key={s.job} title={`${s.status} · entered ${s.count}×`} className={`flex shrink-0 items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] ${s.current ? "border-foreground/70 bg-foreground/5 font-medium" : s.count ? "text-foreground" : "text-muted-foreground"}`}>
        {s.current ? <Loader2 className="size-3 animate-spin" /> : <CircleDot className="size-3" />}{s.job}{s.count > 0 && <span className="text-amber-700 dark:text-amber-400">{s.count}×</span>}
      </li>)}
    </ol>
    {!entries.length && <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">This ticket has not entered a step yet.</p>}
    <ol aria-label="Run entries" className="space-y-2">
      {entries.map((e, i) => <Entry key={e.seq} e={e} prev={i > 0 ? entries[i - 1] : undefined} live={live.has(e.pane ?? "")} clock={clock} attachment={attachment} open={setSelected} openButton={openButton} />)}
    </ol>
    <p className="text-[11px] text-muted-foreground">Prompt, pane and outputs are recorded from now on; older entries (including finished runs) show their status moves, times and results only.</p>
  </div>
  const viewer = selected && <AttachmentViewer key={selected.ref} a={selected} url={url} close={() => setSelected(null)} />

  if (!page) return <>{list}{viewer}</>
  return <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border bg-card">
    <header className="flex h-10 shrink-0 items-center gap-1.5 border-b px-3">
      <Button variant="ghost" size="icon-xs" className="md:hidden" aria-label="Back to home" onClick={() => nav("/")}><ArrowLeft /></Button>
      <Repeat className="size-4 shrink-0 text-amber-600" />
      <span className="shrink-0 text-[11px] font-medium tabular-nums text-muted-foreground">{shortId(run.ticket)}</span>
      <h2 className="min-w-0 truncate text-[13px] font-semibold">{run.title}</h2>
      <span className="hidden shrink-0 text-[11px] text-muted-foreground sm:inline">{run.workflow}</span>
      <a href={`#/workflows/${enc(run.workflow)}`} className="ml-auto inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-teal-700 underline-offset-2 hover:bg-accent hover:underline dark:text-teal-400"><Pencil className="size-3" />Edit workflow</a>
    </header>
    <div className="min-h-0 flex-1 overflow-y-auto p-3">{list}</div>
    {viewer}
  </section>
}

// RunPage is the standalone #/runs/<ticket> view.
export function RunPage({ dir, id }: { dir: string; id: string }) {
  return <RunView dir={dir} id={id} page />
}

function Entry({ e, prev, live, clock, attachment, open, openButton }: {
  e: RunEntry
  prev?: RunEntry
  live: boolean
  clock: (iso?: string) => string
  attachment: (name: string, commentId?: string) => Attachment | undefined
  open: (a: Attachment) => void
  openButton: (a: Attachment, label: string) => ReactNode
}) {
  const chip = resultChip(e.result)
  const outputs = Object.entries(e.outputs ?? {})
  const ciLog = e.exit_code === undefined ? undefined : attachment("ci.log", e.comment_id)
  const files = (e.attachments ?? []).filter(a => a.name !== "ci.log").map(a => attachment(a.name, e.comment_id)).filter((a): a is Attachment => !!a)
  // The first CI line is the exit code, already shown; the rest is the failure summary.
  const ciSummary = e.exit_code === undefined ? "" : (e.comment ?? "").split("\n").slice(1).join("\n").trim()
  return <li className="rounded-lg border bg-card p-2.5 shadow-sm">
    <div className="flex items-center gap-x-2">
      <span className="inline-flex min-w-0 shrink items-center gap-1 text-[13px] font-medium">{e.loop > 1 && <Repeat className="size-3.5 shrink-0 text-amber-600" />}<span className="truncate">{entryLabel(e, prev)}</span></span>
      <span className={`shrink-0 rounded px-1.5 py-px text-[11px] ${chip.cls}`}>{chip.label}</span>
      {(e.role || e.harness) && <span className="inline-flex min-w-0 items-center gap-1 text-[11.5px] text-muted-foreground"><Avatar role={e.role ?? ""} /><span className="truncate">{[e.role, e.harness, e.model].filter(Boolean).join(" · ")}</span></span>}
      <span className="ml-auto shrink-0 text-[11px] tabular-nums text-muted-foreground" title={`${clock(e.start)} → ${e.end ? clock(e.end) : "now"}`}>{clock(e.start)} · {formatDuration(e.duration)}</span>
    </div>
    {(e.route || e.tier) && <p className="mt-1 text-[11.5px] text-muted-foreground">{e.route === "step model" && e.tier ? `tier ${e.tier} · step model` : e.route || `tier ${e.tier}`}</p>}
    {e.pane && <p className="mt-1 text-[11.5px] text-muted-foreground">pane {live ? <a href={`#/agent/${enc(e.pane)}`} className="text-teal-700 underline-offset-2 hover:underline dark:text-teal-400">{e.pane_name || e.pane}</a> : <span>{e.pane_name || e.pane} · closed</span>}</p>}
    {e.reason && e.loop > 1 && <p className="mt-1 line-clamp-3 whitespace-pre-wrap text-[11.5px] text-amber-700 dark:text-amber-400">{e.reason}</p>}
    {ciLog ? <p className="mt-1.5 flex flex-wrap items-center gap-1.5 text-[11.5px]">CI exit <b className={e.exit_code === 0 ? "text-emerald-600" : "text-red-600"}>{e.exit_code}</b>{openButton(ciLog, "ci.log")}</p> : null}
    {ciSummary && <pre className="mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words rounded bg-muted/40 p-1.5 text-[11px]">{ciSummary}</pre>}
    {e.comment && e.exit_code === undefined && <div className="mt-1.5 rounded-md bg-muted/40 p-2 text-[12px]"><Md text={e.comment} /></div>}
    {!!outputs.length && <div className="mt-1.5 flex flex-wrap items-center gap-1.5 text-[11px]">{outputs.map(([k, v]) => {
      const name = v.split("/").filter(Boolean).pop() ?? v
      const a = attachment(name, e.comment_id)
      return a ? <span key={k}>{openButton(a, `${k}=${v}`)}</span> : <code key={k} className="rounded bg-muted/40 px-1.5 py-0.5">{k}={v}</code>
    })}</div>}
    {!!files.length && <AttachmentRows items={files} open={open} />}
    {e.prompt && <details className="mt-1.5">
      <summary className="cursor-pointer text-[11.5px] text-muted-foreground">Prompt sent</summary>
      <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted/40 p-2 font-mono text-[11px]">{e.prompt}</pre>
    </details>}
  </li>
}
