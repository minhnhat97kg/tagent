import { useState } from "react"
import { MessageSquare } from "lucide-react"
import { api, post } from "@/api"
import { Panel } from "@/components/Panel"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { usePoll } from "@/hooks/usePoll"
import { enc } from "@/hooks/useRoute"
import type { Chat, Draft } from "@/types"
export function ChatPanel({dir, onTasks, className}: {dir: string; onTasks: () => void; className?: string}) {
 const poll = usePoll<Chat>(() => api(`/api/chat?dir=${enc(dir)}`), 2000, dir)
 const chat = poll.data?.dir === dir ? poll.data : undefined
 const [text,setText] = useState("")
 const [pending,setPending] = useState(false)
 const [error,setError] = useState("")
 const [edit,setEdit] = useState<{revision: number; draft: Draft | null; command: string} | null>(null)
 const busy = pending || !chat || !chat.enabled || ["running","approving","uncertain"].includes(chat.status)
 const act = async (op: string) => {
  if (!chat || busy) return
  setPending(true); setError("")
  try {
   await post("/api/chat", {dir, op, revision: edit?.revision ?? chat.revision, text, ...(op === "edit" ? {draft: edit?.draft, command: edit?.command} : {})})
   if (op === "send") setText("")
   setEdit(null); poll.reload(); if (op === "create" || op === "start") onTasks()
  } catch (e) {setError((e as Error).message);poll.reload()} finally {setPending(false)}
 }
 return <Panel className={className} icon={<MessageSquare className="text-sky-600" />} title="Chat" bodyClass="flex flex-col">
 {!chat ? <p className="p-2.5 text-xs text-muted-foreground">{poll.error?.message ?? "Loading chat…"}</p> : !chat.enabled ? <p className="p-2.5 text-xs text-muted-foreground">Chat disabled</p> : <>
 <div role="log" className="min-h-20 flex-1 space-y-2 overflow-y-auto p-2.5 text-xs">
 {!chat.messages.length && <p className="text-muted-foreground">Ask: which agents are running? · Draft: a ticket</p>}
 {chat.messages.map((m,i) => <div key={`${m.revision}-${i}`} className={m.role === "user" ? "ml-6 rounded bg-muted p-2" : "mr-4 whitespace-pre-wrap break-words"}><span className="font-medium">{m.role === "user" ? "You" : "Receptionist"}: </span>{m.text}</div>)}
 {chat.response.draft && <div className="rounded border p-2"><strong>{chat.response.draft.title}</strong><p>{chat.response.draft.goal}</p>{chat.response.draft.acceptance.map((a,i) => <p key={i}>• {a}</p>)}<p className="text-muted-foreground">{chat.response.draft.workflow}</p></div>}
 {chat.status === "uncertain" && <p role="alert">Approval interrupted: inspect the project; outcome uncertain.</p>}
 {["running","approving"].includes(chat.status) && <p className="text-muted-foreground">Working…</p>}
 </div>
 {edit && <div className="space-y-1 border-t p-2">
 {edit.draft ? <><Input aria-label="Draft title" value={edit.draft.title} onChange={e => setEdit({...edit,draft:{...edit.draft!,title:e.target.value}})} /><textarea aria-label="Draft goal" className="w-full rounded border p-2 text-xs" value={edit.draft.goal} onChange={e => setEdit({...edit,draft:{...edit.draft!,goal:e.target.value}})} /><textarea aria-label="Acceptance criteria, one per line" className="w-full rounded border p-2 text-xs" value={edit.draft.acceptance.join("\n")} onChange={e => setEdit({...edit,draft:{...edit.draft!,acceptance:e.target.value.split("\n")}})} /><textarea aria-label="Out of scope, one per line" className="w-full rounded border p-2 text-xs" value={edit.draft.outOfScope.join("\n")} onChange={e => setEdit({...edit,draft:{...edit.draft!,outOfScope:e.target.value.split("\n")}})} /><label className="text-xs">Workflow<select className="ml-2 rounded border" value={edit.draft.workflow} onChange={e => setEdit({...edit,draft:{...edit.draft!,workflow:e.target.value}})}><option value="">None</option>{chat.workflows.map(w => <option key={w}>{w}</option>)}</select></label></> : <Input aria-label="Command" value={edit.command} onChange={e => setEdit({...edit,command:e.target.value})} />}
 <Button size="sm" disabled={busy} onClick={() => void act("edit")}>Save</Button><Button size="sm" variant="ghost" onClick={() => setEdit(null)}>Close</Button></div>}
 <div className="flex flex-wrap gap-1 px-2">{chat.actions.map(op => <Button key={op} size="sm" variant="outline" disabled={busy} onClick={() => op === "edit" ? setEdit({revision: chat.revision,draft: chat.response.draft,command:chat.response.command}) : void act(op)}>{op === "create" ? "Create ticket" : op === "start" ? "Create & start" : op}</Button>)}</div>
 <form className="flex gap-1 border-t p-2" onSubmit={e => {e.preventDefault();void act("send")}}><Input aria-label="Chat message" placeholder="Message the receptionist…" value={text} onChange={e => setText(e.target.value)} disabled={busy || chat.status === "proposal"} /><Button size="sm" disabled={busy || !text.trim() || chat.status === "proposal"}>Send</Button></form>
 </>}
 {error && <p role="alert" className="p-2 text-xs text-destructive">{error}</p>}
 </Panel>
}
