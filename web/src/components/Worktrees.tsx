import { act, post } from '@/api'
import { useAppState } from '@/state'
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
export function Worktrees({ onClose }: { onClose: () => void }) {
 const { state } = useAppState()
 const agents = (state?.agents ?? []).filter(a => a.branch)
 return <Dialog open onOpenChange={onClose}><DialogContent><DialogTitle>Agent worktrees</DialogTitle><DialogDescription>Deleting removes the agent window, worktree and branch.</DialogDescription>{!agents.length && <p>No agent worktrees.</p>}{agents.map(a => <div key={a.pane} className="flex items-center gap-2 border-b py-2"><div className="min-w-0 flex-1"><b>{a.branch}</b> · {a.status}<p className="truncate text-xs text-muted-foreground">{a.path}</p></div><Button variant="outline" size="xs" onClick={() => { if (confirm('Delete worktree and branch ' + a.branch + '?')) void act(post('/api/pane/action', { pane: a.pane, data: 'delete-worktree' })) }}>Delete…</Button></div>)}</DialogContent></Dialog>
}
