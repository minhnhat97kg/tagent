import { useEffect, useRef, useState } from "react"
import { ArrowUp, Bot, ChevronDown, ChevronRight, Cpu, MessageSquare, SquarePen, Wrench } from "lucide-react"
import { api, post } from "@/api"
import { Panel } from "@/components/Panel"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Bubble, Composer, Markup, Typing, chatColumn, composerInput, sendButton } from "@/components/ChatParts"
import { usePoll } from "@/hooks/usePoll"
import { enc } from "@/hooks/useRoute"
import type { Chat, Draft } from "@/types"
export function ChatPanel({dir, onTasks, className}: {dir: string; onTasks: () => void; className?: string}) {
 const poll = usePoll<Chat>(() => api(`/api/chat?dir=${enc(dir)}`), 2000, dir)
 const chat = poll.data?.dir === dir ? poll.data : undefined
 const models = usePoll<{model: string; models: string[]; harness?: string}>(() => api("/api/chat/models"), 15000, "models")
 // The receptionist's model is the same setting as Settings > Chat model; the backend (ollama or openai) is Settings > Chat backend.
 const pickModel = async (m: string) => {
  try {await post("/api/config", {Key: "@agent_chat_model", Value: m}); models.reload(); poll.reload()} catch (e) {setError((e as Error).message)}
 }
 const [text,setText] = useState("")
 const [cursor,setCursor] = useState(0)
 const [selected,setSelected] = useState(0)
 const [dismissed,setDismissed] = useState(false)
 const log = useRef<HTMLDivElement>(null)
 const composer = useRef<HTMLTextAreaElement>(null)
 const nearBottom = useRef(true)
 const [pending,setPending] = useState(false)
 const [error,setError] = useState("")
 const [edit,setEdit] = useState<{revision: number; draft: Draft} | null>(null)
 useEffect(() => {if (chat?.status !== "proposal") setEdit(null)},[chat?.status])
 useEffect(() => {setText("");setError("");setEdit(null)},[dir])
 useEffect(() => {if (nearBottom.current) log.current?.scrollTo(0,log.current.scrollHeight)},[chat?.revision])
 const match = text.slice(0,cursor).match(/(?:^|\s)@([^\s`]*)$/)
 const candidates = (chat?.recipients ?? []).flatMap(r => [{token:r.Name || r.Pane,label:`${r.Name || r.Pane} (${r.Pane})`},{token:r.Pane,label:r.Pane},...(r.Role ? [{token:r.Role,label:`Role: ${r.Role}`}]:[])])
 const suggestions = !dismissed && match && (text.slice(0,cursor).match(/`/g)?.length ?? 0)%2===0 ? candidates.filter((r,i,all) => r.token.startsWith(match[1]) && all.findIndex(x => x.token===r.token)===i).slice(0,8) : []
 const complete = (token: string) => {const start=cursor-(match?.[1].length ?? 0)-1;const value=text.slice(0,start)+`@${token} `+text.slice(cursor);setText(value);setCursor(start+token.length+2);setDismissed(true);requestAnimationFrame(() => {composer.current?.focus();composer.current?.setSelectionRange(start+token.length+2,start+token.length+2)})}
 const busy = pending || !chat || !chat.enabled || ["running","approving","uncertain"].includes(chat.status)
 const act = async (op: string) => {
  if (!chat || (busy && op !== "clear")) return
  const acknowledgeUncertain = op === "clear" && chat.status === "uncertain"
  if (acknowledgeUncertain && !window.confirm("Have you inspected the project? Clearing cannot undo an interrupted mutation or establish its outcome.")) return
  setPending(true); setError("")
  try {
   await post("/api/chat", {dir, op, acknowledgeUncertain, revision: edit?.revision ?? chat.revision, text, ...(op === "edit" ? {draft: edit?.draft} : {})})
   if (op === "send" || op === "clear") setText("")
   setEdit(null); poll.reload(); if (op === "create" || op === "start") onTasks()
  } catch (e) {setError((e as Error).message);poll.reload()} finally {setPending(false)}
 }
 const label = (role: string) => role === "user" ? "user" : role === "error" ? "error" : "assistant"
 return <Panel className={className} icon={<MessageSquare className="text-sky-600" />} title="Chat" bodyClass="flex flex-col"
  right={chat?.enabled && <>
  {models.data && <DropdownMenu>
   <DropdownMenuTrigger asChild><Button size="xs" variant="ghost" aria-label={`Model: ${models.data.model}`} className="font-mono text-muted-foreground"><Cpu />{models.data.model}<ChevronDown /></Button></DropdownMenuTrigger>
   <DropdownMenuContent align="end">{models.data.models.map(m => <DropdownMenuItem key={m} onSelect={() => void pickModel(m)}>{m}</DropdownMenuItem>)}{models.data.harness && <DropdownMenuItem disabled className="text-xs">Backend: {models.data.harness} (change in Settings)</DropdownMenuItem>}</DropdownMenuContent>
  </DropdownMenu>}
  <Button size="xs" variant="ghost" disabled={pending || chat.status === "approving"} onClick={() => void act("clear")}><SquarePen />New chat</Button></>}>
 {!chat ? <p className="p-2.5 text-xs text-muted-foreground">{poll.error?.message ?? "Loading chat…"}</p> : !chat.enabled ? <p className="p-2.5 text-xs text-muted-foreground">Chat disabled</p> : <>
 <div role="log" aria-live="polite" ref={log} onScroll={e => {const el=e.currentTarget;nearBottom.current=el.scrollHeight-el.scrollTop-el.clientHeight<64}} className="min-h-20 flex-1 overflow-y-auto">
 <div className={`${chatColumn} space-y-4 px-3 py-4`}>
 {!chat.messages.length && <div className="flex flex-col items-center gap-3 py-10 text-center">
  <span className="flex size-10 items-center justify-center rounded-full border bg-card"><Bot className="size-5" /></span>
  <h2 className="text-base font-semibold">What would you like to do?</h2>
  <p className="max-w-sm text-sm text-muted-foreground">Inspect your project or propose an action. You approve every change.</p>
  <div className="flex flex-wrap justify-center gap-2">{["/agents","/board","Draft a ticket for improving tests"].map(prompt => <button key={prompt} type="button" className="rounded-full border bg-background px-3 py-1 text-xs hover:bg-accent" onClick={() => {setText(prompt);composer.current?.focus()}}>{prompt}</button>)}</div>
 </div>}
 {chat.messages.map((m,i) => m.tool ? <div key={`${m.revision}-${i}`} className="ml-8 space-y-1.5 rounded-xl border bg-muted/30 p-2.5 text-xs">
  <span className="inline-flex items-center gap-1 rounded-full bg-background px-2 py-0.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground"><Wrench className="size-3" />{m.tool.Status}</span>
  {m.tool.Calls.map((c,n) => <details key={n} open className="group rounded-lg border bg-background"><summary className="flex cursor-pointer list-none items-center gap-1.5 px-2 py-1 font-mono [&::-webkit-details-marker]:hidden"><ChevronRight className="size-3 transition-transform group-open:rotate-90" /><span>{c.name}{c.args.pane ? ` → ${c.args.pane}` : ""}</span></summary><pre className="overflow-x-auto border-t p-2 whitespace-pre-wrap">{JSON.stringify(c.args,null,2)}</pre></details>)}
  {m.tool.Output && (m.tool.Calls[0]?.name==="open_view" && m.tool.Output.startsWith("#/") ? <a className="text-sm underline" href={m.tool.Output}>Open view</a> : <pre className="max-h-64 overflow-auto whitespace-pre-wrap">{m.tool.Output}</pre>)}
 </div> : <Bubble key={`${m.revision}-${i}`} who={label(m.role)}>{m.role === "user" ? <span className="whitespace-pre-wrap">{m.text}</span> : <Markup text={m.text} />}</Bubble>)}
 {chat.response.draft && <div className="ml-8 space-y-1 rounded-xl border bg-card p-3 text-sm shadow-sm"><div className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">Ticket draft</div><strong>{chat.response.draft.title}</strong><p>{chat.response.draft.goal}</p>{chat.response.draft.acceptance.map((a,i) => <p key={i} className="text-muted-foreground">• {a}</p>)}{chat.response.draft.workflow && <span className="inline-block rounded-full bg-muted px-2 py-0.5 text-xs">{chat.response.draft.workflow}</span>}</div>}
 {chat.status === "uncertain" && <p role="alert" className="ml-8 rounded-lg border border-amber-500/40 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-400">Approval interrupted: inspect the project; outcome uncertain.</p>}
 {["running","approving"].includes(chat.status) && <Typing label={chat.status === "running" ? "Receptionist is typing…" : "Running approved action…"} />}
 {chat.status === "failed" && <Button size="sm" variant="outline" className="ml-8" onClick={() => setText([...chat.messages].reverse().find(m => m.role==="user")?.text ?? "")}>Restore request to retry</Button>}
 {!!chat.actions.length && <div className="ml-8 flex flex-wrap gap-1.5">{chat.actions.map(op => <Button key={op} size="sm" variant={op === "approve" || op === "create" || op === "start" ? "default" : "outline"} disabled={busy} onClick={() => op === "edit" ? chat.response.draft && setEdit({revision: chat.revision,draft: chat.response.draft}) : void act(op)}>{op === "create" ? "Create ticket" : op === "start" ? "Create & start" : op}</Button>)}</div>}
 </div>
 </div>
 {edit && <div className={`${chatColumn} space-y-1.5 border-t px-3 py-2`}>
 <input aria-label="Draft title" className="w-full rounded-md border bg-background p-2 text-sm" value={edit.draft.title} onChange={e => setEdit({...edit,draft:{...edit.draft,title:e.target.value}})} />
 <textarea aria-label="Draft goal" className="w-full rounded-md border bg-background p-2 text-xs" value={edit.draft.goal} onChange={e => setEdit({...edit,draft:{...edit.draft,goal:e.target.value}})} />
 <textarea aria-label="Acceptance criteria, one per line" className="w-full rounded-md border bg-background p-2 text-xs" value={edit.draft.acceptance.join("\n")} onChange={e => setEdit({...edit,draft:{...edit.draft,acceptance:e.target.value.split("\n")}})} />
 <textarea aria-label="Out of scope, one per line" className="w-full rounded-md border bg-background p-2 text-xs" value={edit.draft.outOfScope.join("\n")} onChange={e => setEdit({...edit,draft:{...edit.draft,outOfScope:e.target.value.split("\n")}})} />
 <div className="flex items-center gap-2"><label className="text-xs">Workflow<select className="ml-2 rounded-md border bg-background p-1" value={edit.draft.workflow} onChange={e => setEdit({...edit,draft:{...edit.draft,workflow:e.target.value}})}><option value="">None</option>{chat.workflows.map(w => <option key={w}>{w}</option>)}</select></label>
 <Button size="sm" className="ml-auto" disabled={busy} onClick={() => void act("edit")}>Save</Button><Button size="sm" variant="ghost" onClick={() => setEdit(null)}>Close</Button></div></div>}
 <Composer onSubmit={() => void act("send")} hint="Enter to send · Shift+Enter for a new line · @ to mention an agent"
  above={suggestions.length>0 && <ul id="chat-mentions" role="listbox" aria-label="Agent mentions" className="absolute inset-x-3 bottom-full z-10 mb-1 rounded-xl border bg-popover p-1 shadow-lg">{suggestions.map((r,i) => <li key={r.token} role="option" aria-selected={i===selected}><button type="button" className={`w-full rounded-lg px-2.5 py-1.5 text-left text-xs ${i===selected ? "bg-accent" : "hover:bg-accent/60"}`} onClick={() => complete(r.token)}>{r.label}</button></li>)}</ul>}>
 <textarea ref={composer} rows={1} aria-label="Chat message" aria-autocomplete="list" aria-controls="chat-mentions" className={composerInput} placeholder="Message the receptionist…" value={text} onSelect={e => setCursor(e.currentTarget.selectionStart)} onChange={e => {setText(e.target.value);setCursor(e.target.selectionStart);setDismissed(false);setSelected(0)}} onKeyDown={e => {if (e.nativeEvent.isComposing) return; if (suggestions.length) {if (e.key==="Escape") {e.preventDefault();setDismissed(true);return} if (e.key==="ArrowDown" || e.key==="ArrowUp") {e.preventDefault();setSelected((selected+(e.key==="ArrowDown" ? 1 : suggestions.length-1))%suggestions.length);return} if ((e.key==="Enter" && !e.shiftKey) || e.key==="Tab") {e.preventDefault();complete(suggestions[selected % suggestions.length].token);return}} if (e.key === "Enter" && !e.shiftKey) {e.preventDefault(); if (text.trim()) void act("send")}}} disabled={busy || chat.status === "proposal"} />
 <Button size="icon" aria-label="Send" className={sendButton} disabled={busy || !text.trim() || chat.status === "proposal"}><ArrowUp /></Button>
 </Composer>
 </>}
 {error && <p role="alert" className="px-3 pb-2 text-xs text-destructive">{error}</p>}
 </Panel>
}
