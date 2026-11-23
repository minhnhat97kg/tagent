import type { Agent, Overview, Project, Task } from '../types'
export type Notice = { id: string; kind: 'waiting' | 'quiet' | 'error' | 'done'; agent: Agent } | { id: string; kind: 'blocked'; task: Task; reason: string; role: string }
export function notifications(agents: Agent[], proj?: Project, overview?: Overview): Notice[] {
 const quiet = new Set(overview?.attention.items?.filter(i => i.kind === 'quiet').map(i => i.pane))
 const out: Notice[] = []
 for (const a of agents) {
  const kind = a.status === 'waiting' ? 'waiting' : a.status === 'error' ? 'error' : a.status === 'done' ? 'done' : a.status === 'running' && (a.stuck || quiet.has(a.pane)) ? 'quiet' : undefined
  if (kind) out.push({id: 'agent:' + a.pane, kind, agent:a})
 }
 for (const task of proj?.tasks ?? []) if (task.status === 'blocked') out.push({id:'task:'+task.id,kind:'blocked',task,reason:proj?.blockedReasons?.[task.id] || task.note || 'Awaiting a decision',role:proj?.workers?.[task.id]?.role || proj?.statuses.find(s=>s.id === task.status)?.role || task.assignees?.[0] || proj?.roles?.[0] || ''})
 return out
}
export function nextNotice(items: Notice[], selected?: string) {
 const i = items.findIndex(n => n.id === selected)
 return items.length ? items[(i + 1) % items.length] : undefined
}
