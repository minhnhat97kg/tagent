import { Board } from "@/components/Board"
import { TasksPanel } from "@/components/panels/TasksPanel"
import { useMedia } from "@/hooks/useMedia"
import { useSize } from "@/hooks/useSize"
import type { Project } from "@/types"

// Widths of the Tasks pane: a list, a list with the ticket beside it, or the board.
const SPLIT_MIN_WIDTH = 480
const BOARD_MIN_WIDTH = 900

// TasksPane is the Tasks pane. It follows its own width, not the window's. The split tier
// needs a desktop window (a phone keeps the list).
export function TasksPane({ proj, dir, reload }: { proj?: Project; dir: string; reload: () => void }) {
  const [ref, { w }] = useSize()
  const desktop = useMedia("(min-width: 768px)")
  const tier = w >= BOARD_MIN_WIDTH ? "board" : w >= SPLIT_MIN_WIDTH && desktop ? "split" : "list"
  return <div ref={ref} className="tasks-pane flex min-h-0 flex-1 flex-col">
    {tier === "board"
      ? <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border bg-card"><Board dir={dir} proj={proj} reload={reload} /></section>
      : <TasksPanel proj={proj} dir={dir} reload={reload} className="min-h-0 flex-1" split={tier === "split"} />}
  </div>
}
