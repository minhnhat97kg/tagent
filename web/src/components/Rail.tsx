import { Bot, Gauge, ListTodo } from "lucide-react"

const ITEM = "flex size-8 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground"

// RailStrip is the collapsed left rail: one icon per panel, click to reopen it.
// It only exists at md+ widths, where the rail itself has room.
export function RailStrip({ tasks, onExpand }: { tasks: boolean; onExpand: () => void }) {
  return (
    <aside className="hidden w-12 shrink-0 flex-col items-center gap-1 rounded-lg border bg-card py-2 md:flex" aria-label="Panels">
      <button className={ITEM} title="Usage" aria-label="Show usage panel" onClick={onExpand}>
        <Gauge className="size-4 text-brand" />
      </button>
      <button className={ITEM} title="Agents" aria-label="Show agents panel" onClick={onExpand}>
        <Bot className="size-4 text-brand" />
      </button>
      {tasks && (
        <button className={ITEM} title="Tickets" aria-label="Show tickets panel" onClick={onExpand}>
          <ListTodo className="size-4" />
        </button>
      )}
    </aside>
  )
}
