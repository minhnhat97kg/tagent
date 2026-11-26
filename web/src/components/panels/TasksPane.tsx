import { useRef, useState } from "react"
import { Board, TaskDrawer } from "@/components/Board"
import { NoPaneChrome, PaneHeader } from "@/components/PaneChrome"
import { TasksPanel } from "@/components/panels/TasksPanel"
import { WorkflowsPane } from "@/components/panels/WorkflowsPane"
import { useSize } from "@/hooks/useSize"
import { cn } from "@/lib/utils"
import type { Project } from "@/types"

// From this width the Tasks pane is the board, whose lanes scroll sideways; narrower, it is the list.
const BOARD_MIN_WIDTH = 600

// TasksPane is the Tasks pane. It follows its own width, not the window's. The open ticket lives
// here and shows in the side drawer, so it stays open when the pane switches between list and board.
export function TasksPane({ proj, dir, reload }: { proj?: Project; dir: string; reload: () => void }) {
  const [ref, { w }] = useSize()
  const [openId, setOpenId] = useState<string>()
  const origin = useRef<HTMLElement | null>(null)
  const open = (id: string, from: HTMLElement) => { origin.current = from; setOpenId(id) }
  return <div ref={ref} className="tasks-pane flex min-h-0 flex-1 flex-col">
    {w >= BOARD_MIN_WIDTH
      ? <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border bg-card"><Board dir={dir} proj={proj} reload={reload} selectedID={openId} onSelect={open} /></section>
      : <TasksPanel proj={proj} dir={dir} reload={reload} className="min-h-0 flex-1" openId={openId} onOpen={open} />}
    <TaskDrawer proj={proj} dir={dir} reload={reload} id={openId} onClose={() => setOpenId(undefined)} onOpen={setOpenId} origin={origin} />
  </div>
}

type Tab = "tasks" | "workflows"

// TasksColumn is the Tasks column on home: Tasks | Workflows in one tab strip. Workflows stays mounted
// once opened, only hidden, so an unsaved draft survives a switch. The strip is the pane's header.
export function TasksColumn({ proj, dir, reload }: { proj?: Project; dir: string; reload: () => void }) {
  const [tab, setTab] = useState<Tab>("tasks")
  const [opened, setOpened] = useState(false)
  const pick = (t: Tab) => { setTab(t); if (t === "workflows") setOpened(true) }
  return <div className="group/pane flex min-h-0 min-w-0 flex-1 flex-col">
    <PaneHeader as="div" pane={tab} className="mb-1 h-8 shrink-0 px-1.5">
      <div role="tablist" aria-label="Tasks or workflows" className="flex min-w-0 flex-1 items-center gap-1">
        {(["tasks", "workflows"] as const).map(t => <button key={t} type="button" role="tab" aria-selected={tab === t} onClick={() => pick(t)}
          className={cn("h-7 rounded-md px-2.5 text-[12.5px] font-semibold capitalize", tab === t ? "bg-accent" : "text-muted-foreground hover:text-foreground")}>{t}</button>)}
      </div>
    </PaneHeader>
    <NoPaneChrome>
      {tab === "tasks" && <TasksPane proj={proj} dir={dir} reload={reload} />}
      {opened && <div className={cn("flex min-h-0 flex-1 flex-col", tab !== "workflows" && "hidden")}><WorkflowsPane dir={dir} /></div>}
    </NoPaneChrome>
  </div>
}
