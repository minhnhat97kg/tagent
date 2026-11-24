import { useState } from 'react'
import { Bell, X } from 'lucide-react'
import { Popover } from 'radix-ui'
import { act, post } from '@/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { KillDialog } from './KillDialog'
import { agentLabel } from '@/lib/status'
import type { Agent } from '@/types'
import type { Notice } from '@/lib/notifications'

const groupOf = (n: Notice) => 'agent' in n ? n.agent.project || n.agent.path.split('/').pop() || 'agents' : 'Tickets'
const TONE: Record<Notice['kind'], { dot: string; text: string }> = {
 waiting: { dot: 'bg-brand', text: 'text-brand-ink' },
 error: { dot: 'bg-destructive', text: 'text-destructive' },
 blocked: { dot: 'bg-amber-500', text: 'text-amber-700 dark:text-amber-400' },
}
const KIND: Record<Notice['kind'], string> = { waiting: 'Needs an answer', error: 'Agent error', blocked: 'Blocked ticket' }

// Reply sends text to the waiting agent, or comments on the blocked ticket, without opening either.
function Reply({ n, dir, done }: { n: Notice; dir: string; done: () => void }) {
 const [text, setText] = useState('')
 const [busy, setBusy] = useState(false)
 const submit = async () => {
  const body = text.trim()
  if (!body) return
  setBusy(true)
  try {
   await act('agent' in n ? post('/api/pane/send', { pane: n.agent.pane, data: body }) : post('/api/tasks', { Dir: dir, Op: 'comment', ID: n.task.id, Text: n.role ? `@${n.role} ${body}` : body }))
   setText(''); done()
  } finally { setBusy(false) }
 }
 return <form className="mt-2.5 flex gap-1.5" onSubmit={e => { e.preventDefault(); void submit() }}>
  <Input value={text} onChange={e => setText(e.target.value)} placeholder="Reply…" aria-label="Reply" className="h-8 flex-1 text-sm" />
  <Button size="sm" type="submit" disabled={busy || !text.trim()}>Send</Button>
 </form>
}

export function Notifications({ items, dir, onOpen }: { items: Notice[]; dir: string; onOpen: (n: Notice) => void }) {
 const [open,setOpen] = useState(false)
 const [kill,setKill] = useState<Agent>()
 const [busy,setBusy] = useState<string>()
 const answer = async (id: string, body: object) => {
  setBusy(id)
  try { await act(post('/api/pane/answer', body)) } finally { setBusy(undefined) }
 }
 const go = (n: Notice) => { setOpen(false); onOpen(n) }
 // Asking for permission needs a click; the bell is the natural one.
 const onOpenChange = (v: boolean) => {
  setOpen(v)
  if (v && typeof Notification !== 'undefined' && Notification.permission === 'default') void Notification.requestPermission()
 }
 const groups = [...new Set(items.map(groupOf))]
 return <>
  <Popover.Root open={open} onOpenChange={onOpenChange}>
   <Popover.Trigger asChild><Button variant="ghost" size="icon-xs" className="relative shrink-0" aria-label="Needs you" title="Needs you" aria-haspopup="dialog"><Bell />{items.length > 0 && <span className={`absolute -right-1 -top-1 min-w-3.5 rounded-full px-0.5 text-[9px] leading-3.5 text-white ${items.some(n => n.kind !== 'waiting') ? 'bg-destructive' : 'bg-brand'}`} aria-label={`${items.length} notifications`}>{items.length}</span>}</Button></Popover.Trigger>
   <Popover.Portal><Popover.Content align="end" sideOffset={8} className="notification-popover z-50 w-[420px] max-w-[calc(100vw-16px)] overflow-y-auto rounded-xl border bg-popover text-popover-foreground shadow-lg" aria-label="Notifications">
    <header className="sticky top-0 z-10 flex items-center justify-between border-b bg-popover px-4 py-3"><h2 className="text-sm font-semibold">Needs you{items.length > 0 && <span className="ml-2 rounded-full bg-brand-soft px-1.5 py-0.5 text-[11px] font-medium text-brand-ink">{items.length}</span>}</h2><Popover.Close asChild><Button size="icon-xs" variant="ghost" aria-label="Close notifications"><X /></Button></Popover.Close></header>
    {!items.length && <p className="px-4 py-8 text-center text-sm text-muted-foreground">Nothing needs you.</p>}
    {groups.map(g => <section key={g}>
     <h3 className="bg-muted/60 px-4 py-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{g}</h3>
     <ul className="divide-y">{items.filter(n => groupOf(n) === g).map(n => <li key={n.id} className="px-4 py-3">
      <button className="block w-full text-left" onClick={() => go(n)}>
       <span className={`flex items-center gap-1.5 text-[11px] font-medium ${TONE[n.kind].text}`}><span className={`size-1.5 rounded-full ${TONE[n.kind].dot}`} />{KIND[n.kind]}</span>
       <strong className="mt-0.5 block text-sm font-semibold">{'agent' in n ? agentLabel(n.agent) : n.task.title || n.task.text.split('\n')[0]}</strong>
       <span className="mt-0.5 line-clamp-3 block whitespace-pre-wrap break-words text-xs leading-relaxed text-muted-foreground">{'agent' in n ? n.agent.dialog?.question || n.agent.wait || n.agent.prompt : n.reason}</span>
      </button>
      <div className="mt-2.5 flex flex-wrap gap-1.5">
       {n.kind === 'waiting' && n.agent.dialog?.options?.map(o => <Button key={o.Key} size="xs" variant="outline" disabled={busy === n.id} onClick={() => void answer(n.id, {pane:n.agent.pane,data:o.Key,dialog:n.agent.dialog})}>{o.Label}</Button>)}
       <Button size="xs" variant="ghost" onClick={() => go(n)}>Open</Button>
       {n.kind === 'error' && <Button size="xs" variant="ghost" className="text-destructive" onClick={() => { setOpen(false); setKill(n.agent) }}>Kill…</Button>}
      </div>
      {(n.kind === 'blocked' || (n.kind === 'waiting' && !n.agent.dialog?.options?.length)) && <Reply n={n} dir={dir} done={() => setOpen(false)} />}
     </li>)}</ul>
    </section>)}
   </Popover.Content></Popover.Portal>
  </Popover.Root>
  {kill && <KillDialog pane={kill.pane} name={agentLabel(kill)} onOpenChange={value => {if (!value) setKill(undefined)}} />}
 </>
}
