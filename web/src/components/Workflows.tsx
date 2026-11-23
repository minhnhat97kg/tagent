import type { Workflow } from '@/types'
export function Workflows({ workflows }: { workflows: Workflow[] }) {
 return <a href="#/workflows">Workflows <span className="board-muted">{workflows.length}</span></a>
}
