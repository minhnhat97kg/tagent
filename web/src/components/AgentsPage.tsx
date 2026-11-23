import { useState } from "react"
import Markdown from "react-markdown"
import { ArrowLeft, Bot, Copy, Eye, Pencil, Plus, Save, Trash2 } from "lucide-react"
import { api, send } from "@/api"
import { usePoll } from "@/hooks/usePoll"
import { enc } from "@/hooks/useRoute"
import { Button } from "@/components/ui/button"
import { editableKey, effortLevels, modelsIn, roleTemplate, usedBySummary } from "@/lib/roles"
import type { Role } from "@/types"

// AgentsPage edits the agent roles the board and the workflows use: harness, model, effort and instructions.
// Model and effort apply to panes started after the save; running panes keep theirs.
export function AgentsPage({ dir }: { dir: string }) {
  const path = "/api/roles?dir=" + enc(dir)
  const poll = usePoll(() => api<Role[]>(path), 2000, dir)
  const roles = poll.data ?? []
  const [draft, setDraft] = useState<Role>()
  const [preview, setPreview] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const saved = roles.find(r => r.name === draft?.name)
  const panes = saved?.panes ?? []
  const dirty = !!draft && !!saved && editableKey(draft) !== editableKey(saved)
  const readOnly = !draft || draft.source === "global"
  const instructionsOff = readOnly || draft?.harness === "pi"

  function open(r?: Role) {
    if (dirty && !confirm("Discard unsaved agent changes?")) return
    setDraft(r && { ...r }); setPreview(false); setError("")
  }
  async function mutate(method: string, body: object) {
    setBusy(true); setError("")
    try { await send(method, path, body); poll.reload(); return true }
    catch (e) { setError((e as Error).message); return false }
    finally { setBusy(false) }
  }
  async function save(body: Role) {
    // pi agents have no instruction file, so the server refuses instructions for them.
    const out = body.harness === "pi" ? { ...body, instructions: "" } : body
    if (await mutate("POST", out)) setDraft({ ...out })
  }
  function create() {
    const name = prompt("Agent name (letters, numbers, - or _)")
    if (!name) return
    if (roles.some(r => r.name === name)) { setError(`Agent ${name} already exists`); return }
    void mutate("POST", { name, harness: "claude", instructions: roleTemplate(name) }).then(ok => {
      if (ok) setDraft({ name, harness: "claude", model: "", effort: "", source: "repo", path: "", instructions: roleTemplate(name), running: 0, panes: [], usedBy: null })
    })
  }
  const models = modelsIn(roles)
  const levels = draft ? effortLevels(draft.harness) : []
  const fieldClass = "w-full rounded border bg-background p-2 text-sm disabled:opacity-60"

  return <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border bg-card">
    <header className="flex h-10 shrink-0 items-center gap-2 border-b px-3">
      {draft && <Button variant="ghost" size="icon-xs" className="md:hidden" aria-label="Back to agents" onClick={() => open(undefined)}><ArrowLeft /></Button>}
      <Bot className="size-4 text-indigo-600" /><h2 className="text-[13px] font-semibold">Agents</h2>
      <Button size="xs" variant="outline" className="ml-auto" disabled={busy} onClick={create}><Plus />New</Button>
    </header>
    <div className="flex min-h-0 flex-1">
      <nav aria-label="Agent list" className={`min-h-0 w-full shrink-0 overflow-auto border-r md:w-60 ${draft ? "hidden md:block" : ""}`}>
        {poll.error ? <p role="alert" className="p-3 text-sm">Couldn't load agents. {poll.error.message}</p> : !poll.data ? <p className="p-3 text-sm text-muted-foreground">Loading…</p> : !roles.length ? <p className="p-3 text-sm text-muted-foreground">No agents yet.</p> : null}
        {roles.map(r => <button key={r.source + r.name} onClick={() => open(r)} aria-current={draft?.name === r.name}
          className={`block w-full border-b px-3 py-2 text-left hover:bg-accent ${draft?.name === r.name ? "bg-accent" : ""}`}>
          <div className="flex items-center gap-1.5 text-[13px] font-medium">{r.name}<span className="text-[10.5px] font-normal text-muted-foreground">{r.source}</span>{r.running > 0 && <span className="ml-auto text-[11px] text-emerald-600">● {r.running}</span>}</div>
          <p className="truncate text-[11.5px] text-muted-foreground">{r.harness}{r.model ? ` · ${r.model}` : ""}</p>
          <p className="line-clamp-2 text-[11px] text-muted-foreground">{usedBySummary(r)}</p>
        </button>)}
      </nav>
      {!draft ? <div className="hidden flex-1 items-center justify-center text-sm text-muted-foreground md:flex">Select an agent to edit its harness, model and instructions.</div> :
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-auto">
          <div className="flex shrink-0 flex-wrap items-center gap-2 border-b px-3 py-1.5">
            <h3 className="mr-auto truncate text-[13px] font-semibold">{draft.name}{dirty && <span className="ml-1 text-amber-600">•</span>}<span className="ml-2 text-[11px] font-normal text-muted-foreground">{draft.source}{draft.path ? ` · ${draft.path}` : ""}</span></h3>
            {draft.source === "global"
              ? <Button size="xs" variant="outline" disabled={busy} onClick={() => void save({ ...draft, source: "repo", path: "" })}><Copy />Copy to repo</Button>
              : <>
                <Button size="xs" variant="outline" disabled={busy || !dirty} onClick={() => void save(draft)}><Save />Save</Button>
                <Button size="xs" variant="ghost" aria-label="Delete agent" disabled={busy} onClick={() => { if (confirm(draft.path ? `Delete ${draft.name}? Its instruction file ${draft.path} is removed too.` : `Delete ${draft.name} from board.yaml?`)) void mutate("DELETE", { name: draft.name }).then(ok => { if (ok) setDraft(undefined) }) }}><Trash2 /></Button>
              </>}
          </div>
          {error && <p role="alert" className="shrink-0 whitespace-pre-wrap border-b bg-destructive/5 px-3 py-1.5 text-xs text-destructive">{error}</p>}
          {draft.source === "global" && <p className="border-b px-3 py-1.5 text-xs text-muted-foreground">Global agent: read-only here. Copy it to the repo to change it.</p>}
          <div className="grid gap-3 p-3 sm:grid-cols-3">
            <label className="text-[12px] font-medium">Harness
              <select aria-label="Harness" className={fieldClass} disabled={readOnly} value={draft.harness} onChange={e => setDraft({ ...draft, harness: e.target.value, effort: effortLevels(e.target.value).includes(draft.effort) ? draft.effort : "" })}>
                {["pi", "claude", "codex"].map(h => <option key={h} value={h}>{h}</option>)}
              </select></label>
            <label className="text-[12px] font-medium">Model
              <input aria-label="Model" list="agent-models" className={fieldClass} disabled={readOnly} value={draft.model} placeholder="default" onChange={e => setDraft({ ...draft, model: e.target.value })} />
              <datalist id="agent-models">{models.map(m => <option key={m} value={m} />)}</datalist></label>
            <label className="text-[12px] font-medium">Effort
              <select aria-label="Effort" className={fieldClass} disabled={readOnly || !levels.length} value={draft.effort} onChange={e => setDraft({ ...draft, effort: e.target.value })}>
                <option value="">default</option>
                {levels.map(l => <option key={l} value={l}>{l}</option>)}
              </select></label>
          </div>
          <p className="px-3 text-[11.5px] text-muted-foreground">Model and effort apply to new panes only. Used by: {usedBySummary(draft)}.</p>
          <div className="px-3 pt-2 text-[12px]">
            <span className="font-medium">Live panes ({panes.length})</span>
            <ul className="mt-1 space-y-0.5">{panes.map(p => <li key={p.pane}><a className="underline" href={`#/agent/${enc(p.pane)}`}>{p.pane} · {p.status}</a></li>)}</ul>
          </div>
          <div className="flex min-h-0 flex-1 flex-col p-3">
            <div className="mb-1 flex items-center gap-2 text-[12px] font-medium">
              Instructions{draft.harness === "pi" && <span className="font-normal text-muted-foreground">· pi agents have no instruction file</span>}
              <Button size="xs" variant="ghost" className="ml-auto" onClick={() => setPreview(!preview)} aria-label={preview ? "Edit instructions" : "Preview instructions"}>{preview ? <><Pencil />Edit</> : <><Eye />Preview</>}</Button>
            </div>
            {preview
              ? <div className="min-h-40 flex-1 space-y-2 overflow-auto rounded border p-3 text-sm [&_code]:font-mono [&_pre]:overflow-auto [&_pre]:rounded [&_pre]:bg-muted [&_pre]:p-2"><Markdown skipHtml>{draft.instructions}</Markdown></div>
              : <textarea aria-label="Instructions" spellCheck={false} disabled={instructionsOff} value={draft.instructions} onChange={e => setDraft({ ...draft, instructions: e.target.value })}
                className="min-h-40 flex-1 resize-none rounded border bg-background p-3 font-mono text-[12px] leading-5 outline-none disabled:opacity-60" />}
          </div>
        </div>}
    </div>
  </section>
}
