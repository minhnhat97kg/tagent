import { useEffect, useRef } from 'react'
import type { Agent, Overview, Project, Task } from '../types'
export type Notice = { id: string; kind: 'waiting' | 'error'; agent: Agent } | { id: string; kind: 'blocked'; task: Task; reason: string; role: string }
// Only what needs the person: an agent waiting on them, an agent in error, a blocked ticket.
// Finished or quiet agents are not shown; the agents list already has them.
export function notifications(agents: Agent[], proj?: Project, _overview?: Overview): Notice[] {
 const out: Notice[] = []
 for (const a of agents) if (a.status === 'waiting' || a.status === 'error') out.push({id: 'agent:' + a.pane, kind: a.status, agent: a})
 for (const task of proj?.tasks ?? []) if (task.status === 'blocked') out.push({id:'task:'+task.id,kind:'blocked',task,reason:proj?.blockedReasons?.[task.id] || task.note || 'Awaiting a decision',role:proj?.workers?.[task.id]?.role || proj?.statuses.find(s=>s.id === task.status)?.role || task.assignees?.[0] || proj?.roles?.[0] || ''})
 return out
}
// Tab title badge "(N) ..." and a browser notification for each item that appears while the page is open.
// ponytail: Notification needs https or localhost, so over plain http (phone via tailscale IP) only the title badge works.
export function useAlerts(items: Notice[], label: (n: Notice) => string) {
 const seen = useRef<Set<string> | undefined>(undefined)
 const base = useRef(document.title.replace(/^\(\d+\) /, ''))
 useEffect(() => { document.title = (items.length ? `(${items.length}) ` : '') + base.current }, [items.length])
 useEffect(() => {
  const ids = new Set(items.map(n => n.id))
  if (seen.current && typeof Notification !== 'undefined' && Notification.permission === 'granted' && document.hidden)
   for (const n of items) if (!seen.current.has(n.id)) new Notification(n.kind === 'error' ? 'Agent error' : 'Needs you', { body: label(n), tag: n.id })
  seen.current = ids
 }, [items]) // eslint-disable-line react-hooks/exhaustive-deps
}
export function nextNotice(items: Notice[], selected?: string) {
 const i = items.findIndex(n => n.id === selected)
 return items.length ? items[(i + 1) % items.length] : undefined
}
