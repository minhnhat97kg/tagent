import { useEffect, useRef, useState } from "react"
import Markdown from "react-markdown"
import { MessageSquare } from "lucide-react"
import { api, post } from "@/api"
import { Panel } from "@/components/Panel"
import { Button } from "@/components/ui/button"
import { usePoll } from "@/hooks/usePoll"
import { enc } from "@/hooks/useRoute"
import type { Chat, Draft } from "@/types"
export function ChatPanel({dir, onTasks, className}: {dir: string; onTasks: () => void; className?: string}) {
 const poll = usePoll<Chat>(() => api(`/api/chat?dir=${enc(dir)}`), 2000, dir)
 const chat = poll.data?.dir === dir ? poll.data : undefined
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
 return <Panel className={className} icon={<MessageSquare className="text-sky-600" />} title="Chat" bodyClass="flex flex-col">
 {!chat ? <p className="p-2.5 text-xs text-muted-foreground">{poll.error?.message ?? "Loading chat…"}</p> : !chat.enabled ? <p className="p-2.5 text-xs text-muted-foreground">Chat disabled</p> : <>
 <div role="log" aria-live="polite" ref={log} onScroll={e => {const el=e.currentTarget;nearBottom.current=el.scrollHeight-el.scrollTop-el.clientHeight<64}} className="min-h-20 flex-1 space-y-4 overflow-y-auto p-4 text-sm">
 {!chat.messages.length && <div className="mx-auto max-w-lg space-y-3 py-8"><h2 className="text-lg font-semibold">What would you like to do?</h2><p className="text-muted-foreground">Inspect your project or propose an action. You approve every change.</p>{["/agents","/board","Draft a ticket for improving tests"].map(prompt => <Button key={prompt} variant="outline" className="mr-2" onClick={() => setText(prompt)}>{prompt}</Button>)}</div>}
 {chat.messages.map((m,i) => <div key={`${m.revision}-${i}`} className={`max-w-[92%] rounded-xl p-3 break-words ${m.role === "user" ? "ml-auto bg-primary text-primary-foreground whitespace-pre-wrap" : "mr-auto bg-muted/60"}`}><div className="mb-1 text-xs font-medium opacity-70">{m.role === "user" ? "You" : m.role === "error" ? "Error" : "Receptionist"}</div>{m.tool ? <div className="space-y-2 rounded border border-border p-2"><strong className="text-xs uppercase">{m.tool.Status}</strong>{m.tool.Calls.map((c,n) => <details key={n} open><summary className="font-mono text-xs">{c.name}{c.args.pane ? ` → ${c.args.pane}` : ""}</summary><pre className="overflow-x-auto whitespace-pre-wrap text-xs">{JSON.stringify(c.args,null,2)}</pre></details>)}{m.tool.Output && (m.tool.Calls[0]?.name==="open_view" && m.tool.Output.startsWith("#/") ? <a className="text-sm underline" href={m.tool.Output}>Open view</a> : <pre className="whitespace-pre-wrap text-xs">{m.tool.Output}</pre>)}</div> : m.role === "user" ? m.text : <div className="space-y-2 [&_a]:underline [&_code]:font-mono [&_pre]:overflow-auto [&_pre]:rounded [&_pre]:bg-background [&_pre]:p-2 [&_ul]:list-disc [&_ul]:pl-5"><Markdown skipHtml>{m.text}</Markdown></div>}</div>)}
 {chat.response.draft && <div className="rounded border p-2"><strong>{chat.response.draft.title}</strong><p>{chat.response.draft.goal}</p>{chat.response.draft.acceptance.map((a,i) => <p key={i}>• {a}</p>)}<p className="text-muted-foreground">{chat.response.draft.workflow}</p></div>}
 {chat.status === "uncertain" && <p role="alert">Approval interrupted: inspect the project; outcome uncertain.</p>}
 {["running","approving"].includes(chat.status) && <p role="status" className="animate-pulse text-muted-foreground">{chat.status === "running" ? "Receptionist is typing…" : "Running approved action…"}</p>}
 {chat.status === "failed" && <Button size="sm" variant="outline" onClick={() => setText([...chat.messages].reverse().find(m => m.role==="user")?.text ?? "")}>Restore request to retry</Button>}
 </div>
 {edit && <div className="space-y-1 border-t p-2">
<><input aria-label="Draft title" className="w-full rounded border bg-background p-2 text-sm" value={edit.draft.title} onChange={e => setEdit({...edit,draft:{...edit.draft,title:e.target.value}})} /><textarea aria-label="Draft goal" className="w-full rounded border p-2 text-xs" value={edit.draft.goal} onChange={e => setEdit({...edit,draft:{...edit.draft,goal:e.target.value}})} /><textarea aria-label="Acceptance criteria, one per line" className="w-full rounded border p-2 text-xs" value={edit.draft.acceptance.join("\n")} onChange={e => setEdit({...edit,draft:{...edit.draft,acceptance:e.target.value.split("\n")}})} /><textarea aria-label="Out of scope, one per line" className="w-full rounded border p-2 text-xs" value={edit.draft.outOfScope.join("\n")} onChange={e => setEdit({...edit,draft:{...edit.draft,outOfScope:e.target.value.split("\n")}})} /><label className="text-xs">Workflow<select className="ml-2 rounded border" value={edit.draft.workflow} onChange={e => setEdit({...edit,draft:{...edit.draft,workflow:e.target.value}})}><option value="">None</option>{chat.workflows.map(w => <option key={w}>{w}</option>)}</select></label></>
 <Button size="sm" disabled={busy} onClick={() => void act("edit")}>Save</Button><Button size="sm" variant="ghost" onClick={() => setEdit(null)}>Close</Button></div>}
 <div className="flex flex-wrap gap-1 px-2"><Button size="sm" variant="ghost" disabled={pending || chat.status === "approving"} onClick={() => void act("clear")}>New chat</Button>{chat.actions.map(op => <Button key={op} size="sm" variant="outline" disabled={busy} onClick={() => op === "edit" ? chat.response.draft && setEdit({revision: chat.revision,draft: chat.response.draft}) : void act(op)}>{op === "create" ? "Create ticket" : op === "start" ? "Create & start" : op}</Button>)}</div>
 {suggestions.length>0 && <ul id="chat-mentions" role="listbox" aria-label="Agent mentions" className="mx-2 rounded border bg-popover p-1">{suggestions.map((r,i) => <li key={r.token} role="option" aria-selected={i===selected}><button type="button" className={`w-full rounded p-2 text-left text-xs ${i===selected ? "bg-accent" : ""}`} onClick={() => complete(r.token)}>{r.label}</button></li>)}</ul>}
 <form className="flex gap-2 border-t p-3" onSubmit={e => {e.preventDefault();void act("send")}}><textarea ref={composer} aria-label="Chat message" aria-autocomplete="list" aria-controls="chat-mentions" className="min-h-16 flex-1 resize-y rounded border bg-background p-2 text-sm" placeholder="Message the receptionist…" value={text} onSelect={e => setCursor(e.currentTarget.selectionStart)} onChange={e => {setText(e.target.value);setCursor(e.target.selectionStart);setDismissed(false);setSelected(0)}} onKeyDown={e => {if (e.nativeEvent.isComposing) return; if (suggestions.length) {if (e.key==="Escape") {e.preventDefault();setDismissed(true);return} if (e.key==="ArrowDown" || e.key==="ArrowUp") {e.preventDefault();setSelected((selected+(e.key==="ArrowDown" ? 1 : suggestions.length-1))%suggestions.length);return} if ((e.key==="Enter" && !e.shiftKey) || e.key==="Tab") {e.preventDefault();complete(suggestions[selected % suggestions.length].token);return}} if (e.key === "Enter" && !e.shiftKey) {e.preventDefault(); if (text.trim()) void act("send")}}} disabled={busy || chat.status === "proposal"} /><Button size="sm" disabled={busy || !text.trim() || chat.status === "proposal"}>Send</Button></form>
 </>}
 {error && <p role="alert" className="p-2 text-xs text-destructive">{error}</p>}
 </Panel>
}
