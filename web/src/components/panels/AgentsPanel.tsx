import { act, post } from "@/api"
import { Bot, Folder, ListTodo, ListTree } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Panel } from "@/components/Panel"
import { StatusDot } from "@/components/StatusDot"
import { useMedia } from "@/hooks/useMedia"
import { useSize } from "@/hooks/useSize"
import { ago, agentLabel, baseName, isActive, shortHarness, shortPrompt, taskOf } from "@/lib/status"
import { cn } from "@/lib/utils"
import { useState, Fragment } from "react"
import type { Agent } from "@/types"

const projOf = (a: Agent) => a.project || baseName(a.path)

// AgentsPanel lists agents in the order useKeys walks them (grouped by project).
// Under 120px of height (five rows) it shows only the counts; the height tier is desktop only.
export function AgentsPanel({ ordered, byTask, onToggleTask, onSelect, selPane, className }: { ordered: Agent[]; byTask: boolean; onToggleTask: () => void; onSelect: (pane: string) => void; selPane?: string; className?: string }) {
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
      className="@container min-h-0 flex-1"
      icon={<Bot className="text-brand" />}
      title="Agents"
      right={<>{/* A narrow pane drops the link: maximizing it opens the same page. */}<a href="#/agents" className="underline @max-[17rem]:hidden">Definitions</a><Button size="icon-xs" variant={byTask ? "secondary" : "ghost"} className="pointer-coarse:size-7" aria-label="By task" aria-pressed={byTask} title="Group agents by task" onClick={onToggleTask}><ListTree className="size-3.5" /></Button></>}
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
              <h3 className="flex items-center gap-2 px-2.5 pt-2 pb-0.5 text-[11px] font-semibold text-muted-foreground">
                <Folder aria-hidden className="size-3.5 shrink-0" />{projOf(a)}
              </h3>
            )}
            {byTask && taskOf(a) && (i === 0 || projOf(ordered[i - 1]!) !== projOf(a) || taskOf(ordered[i - 1]!) !== taskOf(a)) && (
              <h4 className="flex items-center gap-2 px-2.5 pt-1.5 pb-0.5 text-[10.5px] font-medium text-muted-foreground"><ListTodo aria-hidden className="size-3.5 shrink-0" />Task #{taskOf(a)}</h4>
            )}
            <div
              role="button"
              tabIndex={0}
              className={cn(
                // A narrow pane gives the name the whole line and puts the prompt under it.
                "flex min-h-8 cursor-pointer flex-wrap items-center gap-x-2 px-2.5 py-1 text-[12.5px] hover:bg-accent @md:h-8 @md:flex-nowrap @md:py-0",
                selPane === a.pane && "bg-accent",
              )}
              onClick={() => {
                onSelect(a.pane)
                if (waitingDialog) setOpen(isOpen ? null : a.pane)
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" && e.target === e.currentTarget) e.currentTarget.click()
              }}
            >
              <StatusDot status={a.status} />
              <span className="min-w-0 flex-1 truncate font-medium @md:max-w-[40%] @md:flex-none @md:shrink-0">{agentLabel(a)}</span>
              {/* An unnamed agent's label already is its harness. */}
              {a.name && <span title={a.agent} className="shrink-0 rounded border px-1 py-0.5 font-mono text-[10px] leading-none text-muted-foreground">{shortHarness(a.agent)}</span>}
              {a.hidden && <span title="Background agent" className="shrink-0 rounded bg-muted px-1 py-0.5 text-[10px] leading-none text-muted-foreground">bg</span>}
              <span
                className={cn(
                  "order-last min-w-0 basis-full truncate pl-5.5 text-[12px] @md:order-none @md:flex-1 @md:basis-0 @md:pl-0",
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
