import { useState } from "react"
import { act, post } from "@/api"
import { Input } from "@/components/ui/input"
import { Panel } from "@/components/Panel"
import { TaskDialog } from "@/components/TaskDialog"
import { cn } from "@/lib/utils"
import type { Project } from "@/types"

const GLYPH: Record<string, { glyph: string; cls: string }> = {
  todo: { glyph: "○", cls: "text-muted-foreground" },
  doing: { glyph: "◐", cls: "text-primary" },
  blocked: { glyph: "✖", cls: "text-red-500" },
  done: { glyph: "✔", cls: "text-green-600" },
}

export function TasksPanel({ proj, dir, reload, className }: { proj?: Project; dir: string; reload: () => void; className?: string }) {
  const tasks = proj?.tasks ?? []
  const done = tasks.filter((t) => t.status === "done").length
  const [adding, setAdding] = useState(false)
  const [draft, setDraft] = useState("")
  const [openId, setOpenId] = useState<string | null>(null)
  const openTask = tasks.find((t) => t.id === openId)

  const add = () => {
    const text = draft.trim()
    if (!text) return
    void act(
      post("/api/tasks", { Dir: dir, Op: "add", Text: text }).then(() => {
        setDraft("")
        reload()
      }),
    )
  }

  return (
    <Panel
      className={className}
      icon="☷"
      title={`Tasks · ${done}/${tasks.length}`}
      right={
        <button type="button" className="rounded px-1 hover:bg-accent" aria-label="Add task" onClick={() => setAdding((a) => !a)}>
          +
        </button>
      }
      bodyClass="py-1"
    >
      {adding && (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            add()
          }}
          className="border-b px-2.5 py-1.5"
        >
          <Input
            autoFocus
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && setAdding(false)}
            placeholder="Add a task…"
            className="h-7 text-xs"
          />
        </form>
      )}
      {!tasks.length && !adding && <p className="px-2.5 py-2 text-xs text-muted-foreground">No tasks.</p>}
      {tasks.map((t) => {
        const g = GLYPH[t.status] ?? GLYPH.todo!
        return (
          <button
            key={t.id}
            type="button"
            onClick={() => setOpenId(t.id)}
            className="flex w-full items-start gap-2 px-2.5 py-1 text-left text-xs hover:bg-accent"
          >
            <span className={cn("w-3 shrink-0 text-center", g.cls)}>{g.glyph}</span>
            <span className={cn("min-w-0 flex-1 truncate", t.status === "done" && "line-through opacity-60")}><span className="block truncate">{t.title || t.text.split("\n")[0]}</span><span className="block text-muted-foreground">{[t.status, t.workflow, `#${t.comments?.length ?? 0}`].filter(Boolean).join(" · ")}</span></span>
          </button>
        )
      })}
      {openTask && <TaskDialog task={openTask} dir={dir} onChange={reload} onClose={() => setOpenId(null)} />}
    </Panel>
  )
}
