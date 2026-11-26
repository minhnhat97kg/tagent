import { act, post } from "@/api"
import { Bot } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Panel } from "@/components/Panel"
import { StatusDot } from "@/components/StatusDot"
import { enc, nav } from "@/hooks/useRoute"
import { useMedia } from "@/hooks/useMedia"
import { useSize } from "@/hooks/useSize"
import { ago, agentLabel, baseName, isActive, shortPrompt, taskOf } from "@/lib/status"
import { cn } from "@/lib/utils"
import { useState, Fragment } from "react"
import type { Agent } from "@/types"

const projOf = (a: Agent) => a.project || baseName(a.path)

// AgentsPanel lists agents in the order useKeys walks them (grouped by project).
// Under 120px of height (five rows) it shows only the counts; the height tier is desktop only.
export function AgentsPanel({ ordered, byTask, onToggleTask, selPane, className }: { ordered: Agent[]; byTask: boolean; onToggleTask: () => void; selPane?: string; className?: string }) {
  const [open, setOpen] = useState<string | null>(null)
  const multi = new Set(ordered.map(projOf)).size > 1
  const [ref, { h }] = useSize()
  const desktop = useMedia("(min-width: 768px)")
  const short = desktop && h > 0 && h < 120
  const waiting = ordered.filter(a => a.status === "waiting").length
  const working = ordered.filter(a => isActive(a.status)).length

  return (
    <div ref={ref} className={cn("flex min-h-0 flex-col", className)}>
    <Panel
      className="min-h-0 flex-1"
      icon={<Bot className="text-brand" />}
      title="Agents"
      right={<><a href="#/agents" className="underline">Definitions</a><Button size="xs" variant={byTask ? "secondary" : "ghost"} aria-pressed={byTask} title="Group agents by task" onClick={onToggleTask}>By task</Button></>}
      bodyClass="py-1"
    >
      {short ? <p className="px-2.5 py-2 text-xs text-muted-foreground">{ordered.length ? `${ordered.length} agents, ${waiting} waiting, ${working} working` : "No agents. Press + New agent."}</p> : <>
      {!ordered.length && <p className="px-2.5 py-2 text-xs text-muted-foreground">No agents. Press + New agent.</p>}
      {ordered.map((a, i) => {
        const waitingDialog = a.status === "waiting" && !!a.dialog
        const isOpen = open === a.pane
        return (
          <Fragment key={a.pane}>
            {multi && (i === 0 || projOf(ordered[i - 1]!) !== projOf(a)) && (
              <h3 className="px-2.5 pt-2 pb-0.5 text-[11px] font-semibold text-muted-foreground">
                {projOf(a)}
              </h3>
            )}
            {byTask && taskOf(a) && (i === 0 || projOf(ordered[i - 1]!) !== projOf(a) || taskOf(ordered[i - 1]!) !== taskOf(a)) && (
              <h4 className="px-2.5 pt-1.5 pb-0.5 text-[10.5px] font-medium text-muted-foreground">Task #{taskOf(a)}</h4>
            )}
            <div
              role="button"
              tabIndex={0}
              className={cn(
                "flex h-8 cursor-pointer items-center gap-2 px-2.5 text-[12.5px] hover:bg-accent",
                selPane === a.pane && "bg-accent",
              )}
              onClick={() => {
                nav("/agent/" + enc(a.pane))
                if (waitingDialog) setOpen(isOpen ? null : a.pane)
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" && e.target === e.currentTarget) e.currentTarget.click()
              }}
            >
              <StatusDot status={a.status} />
              <span className="max-w-[40%] shrink-0 truncate font-medium">{agentLabel(a)}</span>
              {a.hidden && <span title="Background agent" className="shrink-0 rounded bg-muted px-1 py-0.5 text-[10px] leading-none text-muted-foreground">bg</span>}
              <span
                className={cn(
                  "min-w-0 flex-1 truncate text-[12px]",
                  waitingDialog ? "text-amber-600 dark:text-amber-400" : "text-muted-foreground",
                )}
              >
                {waitingDialog ? "! " + a.dialog!.question : shortPrompt(a.prompt)}
              </span>
              {a.started ? <span className="shrink-0 text-[11px] tabular-nums text-muted-foreground">{ago(a.started)}</span> : null}
            </div>
            {(a.subagents ?? []).map((s) => (
              <div key={s} className="pl-7 text-[11px] text-muted-foreground">
                └ {s}
              </div>
            ))}
            {isOpen && waitingDialog && (
              <div className="flex flex-wrap gap-1 px-7 pb-2">
                {(a.dialog!.options ?? []).map((o, j) => (
                  <Button
                    key={o.Key}
                    size="xs"
                    variant={j === 0 ? "default" : "outline"}
                    onClick={() => {
                      setOpen(null)
                      void act(post("/api/pane/answer", { pane: a.pane, data: o.Key }))
                    }}
                  >
                    {o.Label}
                  </Button>
                ))}
              </div>
            )}
          </Fragment>
        )
      })}
      </>}
    </Panel>
    </div>
  )
}
