import { useLayoutEffect, useRef, useState } from "react"
import { Board } from "@/components/Board"
import { TasksPanel } from "@/components/panels/TasksPanel"
import type { Project } from "@/types"

// The board needs about 900px of pane width; a narrower pane lists the tasks.
const BOARD_MIN_WIDTH = 900

// TasksPane is the Tasks pane: the board when the pane is wide enough, else the list. It follows its own width, not the window's.
export function TasksPane({ proj, dir, reload }: { proj?: Project; dir: string; reload: () => void }) {
  const ref = useRef<HTMLDivElement>(null)
  const [wide, setWide] = useState(false)
  // Layout effect: the width is read before the first paint, so the list never flashes before the board.
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => setWide(el.clientWidth >= BOARD_MIN_WIDTH)
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    measure()
    return () => ro.disconnect()
  }, [])
  return <div ref={ref} className="tasks-pane flex min-h-0 flex-1 flex-col">
    {wide
      ? <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border bg-card"><Board dir={dir} proj={proj} reload={reload} /></section>
      : <TasksPanel proj={proj} dir={dir} reload={reload} className="min-h-0 flex-1" />}
  </div>
}
