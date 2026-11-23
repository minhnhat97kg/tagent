import { useState } from 'react'
import { Bell } from 'lucide-react'
import { Popover } from 'radix-ui'
import { act, post } from '@/api'
import { Button } from '@/components/ui/button'
import { KillDialog } from './KillDialog'
import { agentLabel } from '@/lib/status'
import type { Agent } from '@/types'
import type { Notice } from '@/lib/notifications'

export function Notifications({ items, onOpen, onReply }: { items: Notice[]; onOpen: (n: Notice) => void; onReply: (n: Extract<Notice,{kind:'blocked'}>) => void }) {
 const [open,setOpen] = useState(false)
 const [kill,setKill] = useState<Agent>()
 const [busy,setBusy] = useState<string>()
 const urgent = items.some(n => n.kind === 'error' || n.kind === 'blocked')
 const run = async (id: string, path: string, body: object) => {
  setBusy(id)
  try { await act(post(path,body)) } finally { setBusy(undefined) }
 }
 const go = (n: Notice) => { setOpen(false); onOpen(n) }
 return <>
  <Popover.Root open={open} onOpenChange={setOpen}>
   <Popover.Trigger asChild><Button variant="ghost" size="icon-xs" className="relative shrink-0" aria-label="Needs you" title="Needs you" aria-haspopup="dialog"><Bell />{items.length > 0 && <span className={`absolute -right-1 -top-1 min-w-3.5 rounded-full px-0.5 text-[9px] leading-3.5 text-white ${urgent ? 'bg-red-600' : 'bg-amber-600'}`} aria-label={`${items.length} notifications`}>{items.length}</span>}</Button></Popover.Trigger>
   <Popover.Portal><Popover.Content align="end" sideOffset={8} className="notification-popover z-50 w-[420px] max-w-[calc(100vw-16px)] overflow-y-auto rounded-lg border bg-popover p-3 text-popover-foreground shadow-lg" aria-label="Notifications">
    <header className="mb-2 flex items-center justify-between"><h2 className="text-[13px] font-semibold">Needs you</h2><Popover.Close asChild><Button size="xs" variant="ghost" aria-label="Close notifications">×</Button></Popover.Close></header>
    {!items.length && <p className="p-2 text-xs text-muted-foreground">Nothing needs you.</p>}
    <ul className="space-y-2">{items.map(n => <li key={n.id} className="rounded-md border p-2">
     <button className="block w-full text-left hover:bg-accent" onClick={() => go(n)}>
      <span className="text-[11px] text-muted-foreground">{n.kind === 'quiet' ? 'Stuck' : n.kind === 'done' ? 'Finished' : n.kind === 'waiting' ? 'Approval needed' : n.kind === 'blocked' ? 'Blocked ticket' : 'Agent error'}</span>
      <strong className="block text-xs">{'agent' in n ? agentLabel(n.agent) : n.task.title || n.task.text.split('\n')[0]}</strong>
      <span className="block whitespace-pre-wrap break-words text-xs text-muted-foreground">{'agent' in n ? n.agent.dialog?.question || n.agent.wait || n.agent.prompt : n.reason}</span>
     </button>
     <div className="mt-2 flex flex-wrap gap-1">
      {n.kind === 'waiting' && n.agent.dialog?.options?.map(o => <Button key={o.Key} size="xs" variant="outline" disabled={busy === n.id} onClick={() => void run(n.id,'/api/pane/answer',{pane:n.agent.pane,data:o.Key,dialog:n.agent.dialog})}>{o.Label}</Button>)}
      <Button size="xs" variant="outline" onClick={() => go(n)}>Open</Button>
      {(n.kind === 'quiet' || n.kind === 'error') && <Button size="xs" variant="outline" onClick={() => { setOpen(false); setKill(n.agent) }}>Kill…</Button>}
      {n.kind === 'done' && <Button size="xs" variant="outline" disabled={busy === n.id} onClick={() => void run(n.id,'/api/pane/action',{pane:n.agent.pane,data:'dismiss'})}>Dismiss</Button>}
      {n.kind === 'blocked' && <Button size="xs" onClick={() => {setOpen(false);onReply(n)}}>Reply</Button>}
     </div>
    </li>)}</ul>
   </Popover.Content></Popover.Portal>
  </Popover.Root>
  {kill && <KillDialog pane={kill.pane} name={agentLabel(kill)} onOpenChange={value => {if (!value) setKill(undefined)}} />}
 </>
}
