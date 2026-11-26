import type { ReactNode } from "react"
import { Maximize2 } from "lucide-react"
import { SplitPane } from "@/components/SplitPane"
import { DEFAULT_PANES, type PaneSizes } from "@/lib/paneLayout"
import { useMedia } from "@/hooks/useMedia"
import { cn } from "@/lib/utils"

export type PhoneTab = "agents" | "chat" | "tasks"
const TABS: { id: PhoneTab; label: string }[] = [{ id: "agents", label: "Agents" }, { id: "chat", label: "Chat" }, { id: "tasks", label: "Tasks" }]
const MIN_COLS = [220, 220, 280]
const MIN_LEFT = [128, 0]

// HomeLayout is the home view. On desktop: Agents (over Usage) | Chat | Tasks, split by dividers that
// are dragged or moved with the arrow keys. A phone shows one of the three as tabs.
export function HomeLayout({ saved, onChange, compact, tab, onTab, panes }: {
  saved?: PaneSizes
  compact?: boolean // tighter spacing only; the layout is the same
  onChange: (sizes: PaneSizes) => void
  tab: PhoneTab
  onTab: (tab: PhoneTab) => void
  panes: { agents: ReactNode; usage: ReactNode; chat: ReactNode; tasks: ReactNode }
}) {
  const desktop = useMedia("(min-width: 768px)")
  const sizes = saved ?? DEFAULT_PANES

  if (!desktop) return <div className="flex min-h-0 flex-1 flex-col gap-2">
    <div role="tablist" aria-label="Home panes" className="flex shrink-0 gap-1 rounded-lg border bg-card p-0.5">
      {TABS.map(t => <button key={t.id} type="button" role="tab" aria-selected={tab === t.id} onClick={() => onTab(t.id)}
        className={cn("h-8 flex-1 rounded-md text-[12.5px] font-medium", tab === t.id ? "bg-accent" : "text-muted-foreground")}>{t.label}</button>)}
    </div>
    {tab === "agents" && <>
      <div className="max-h-[30%] min-h-0 overflow-y-auto">{panes.usage}</div>
      <div className="flex min-h-0 flex-1 flex-col">{panes.agents}</div>
    </>}
    {tab === "chat" && <div className="flex min-h-0 flex-1 flex-col">{panes.chat}</div>}
    {tab === "tasks" && <div className="flex min-h-0 flex-1 flex-col">{panes.tasks}</div>}
  </div>

  const collapsed = sizes.left[1] === 0
  return <SplitPane axis="x" sizes={sizes.cols} defaults={DEFAULT_PANES.cols} mins={MIN_COLS} labels={["Agents and Chat", "Chat and Tasks"]} compact={compact}
    onChange={cols => onChange({ ...sizes, cols })}>
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <SplitPane axis="y" sizes={sizes.left} defaults={DEFAULT_PANES.left} mins={MIN_LEFT} collapsible={1} labels={["Agents over Usage"]} compact={compact}
        onChange={left => onChange({ ...sizes, left })}>
        <Slot id="agents">{panes.agents}</Slot>
        {collapsed ? null : <Slot id="usage">{panes.usage}</Slot>}
      </SplitPane>
      {/* A shut Usage has no height, so its reopen button sits under the column. */}
      {collapsed && <button type="button" aria-label="Show usage" title="Show usage" onClick={() => onChange({ ...sizes, left: DEFAULT_PANES.left })}
        className="h-8 shrink-0 border-t px-2.5 text-left text-[12.5px] font-semibold hover:bg-accent">Usage</button>}
    </div>
    <Slot id="chat">{panes.chat}</Slot>
    {panes.tasks}
  </SplitPane>
}

// Slot is one desktop pane with its corner link to #/pane/<id>, which maximizes it.
function Slot({ id, children }: { id: string; children: ReactNode }) {
  return <div data-pane={id} className="group/pane relative flex min-h-0 min-w-0 flex-1 flex-col">
    {children}
    <a href={`#/pane/${id}`} aria-label={`Maximize ${id} pane`} title="Maximize"
      className="absolute bottom-0 left-0 z-10 flex size-4 items-center justify-center rounded-tr-md text-muted-foreground hover:text-foreground">
      <Maximize2 className="size-3" />
    </a>
  </div>
}
