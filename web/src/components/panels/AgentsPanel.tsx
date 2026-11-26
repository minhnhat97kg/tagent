import { act, post } from "@/api"
import { Bot, ChevronDown, ChevronRight, Folder, ListTodo, ListTree, type LucideIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Panel } from "@/components/Panel"
import { StatusDot } from "@/components/StatusDot"
import { useMedia } from "@/hooks/useMedia"
import { useSize } from "@/hooks/useSize"
import { ago, agentLabel, baseName, isActive, shortHarness, shortPrompt, taskOf } from "@/lib/status"
import { cn } from "@/lib/utils"
import { useState, Fragment, type ReactNode } from "react"
import type { Agent } from "@/types"

const projOf = (a: Agent) => a.project || baseName(a.path)
const SHUT = "tagent.shutGroups"
const loadShut = (): string[] => { try { const v: unknown = JSON.parse(localStorage.getItem(SHUT) ?? "[]"); return Array.isArray(v) ? v : [] } catch { return [] } }

// Group is a group's header: it folds the group away, and then still says how many agents it holds and how many wait.
function Group({ icon: Icon, open, agents, onToggle, className, children }: { icon: LucideIcon; open: boolean; agents: Agent[]; onToggle: () => void; className: string; children: ReactNode }) {
  const waiting = agents.filter(a => a.status === "waiting").length
  const Chevron = open ? ChevronDown : ChevronRight
  return <h3 className={cn("font-semibold text-muted-foreground", className)}>
    <button type="button" aria-expanded={open} onClick={onToggle} className="flex w-full items-center gap-2 px-2.5 py-1 text-left hover:bg-accent pointer-coarse:py-2">
      <Icon aria-hidden className="size-3.5 shrink-0" />
      <span className="min-w-0 truncate">{children}</span>
      {!open && <span className="shrink-0 font-normal">{agents.length}{waiting > 0 && <span className="text-amber-600 dark:text-amber-400"> · {waiting} waiting</span>}</span>}
      <Chevron aria-hidden className="ml-auto size-3.5 shrink-0" />
    </button>
  </h3>
}

// AgentsPanel lists agents in the order useKeys walks them (grouped by project).
// Under 120px of height (five rows) it shows only the counts; the height tier is desktop only.
export function AgentsPanel({ ordered, byTask, onToggleTask, onSelect, selPane, className }: { ordered: Agent[]; byTask: boolean; onToggleTask: () => void; onSelect: (pane: string) => void; selPane?: string; className?: string }) {
  const [open, setOpen] = useState<string | null>(null)
  const [shut, setShut] = useState(loadShut)
  const toggle = (key: string) => {
    const next = shut.includes(key) ? shut.filter(k => k !== key) : [...shut, key]
    setShut(next)
    try { localStorage.setItem(SHUT, JSON.stringify(next)) } catch { /* not remembered in private mode */ }
  }
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
        const project = projOf(a), task = byTask ? taskOf(a) : ""
        const newProject = i === 0 || projOf(ordered[i - 1]!) !== project
        const projectKey = project, taskKey = `${project}#${task}`
        const projectShut = multi && shut.includes(projectKey)
        const taskShut = !!task && shut.includes(taskKey)
        return (
          <Fragment key={a.pane}>
            {multi && newProject && (
              <Group icon={Folder} className="pt-1 text-[11px]" open={!projectShut} onToggle={() => toggle(projectKey)} agents={ordered.filter(x => projOf(x) === project)}>{project}</Group>
            )}
            {!projectShut && task && (newProject || taskOf(ordered[i - 1]!) !== task) && (
              <Group icon={ListTodo} className="text-[10.5px] font-medium" open={!taskShut} onToggle={() => toggle(taskKey)} agents={ordered.filter(x => projOf(x) === project && taskOf(x) === task)}>Task #{task}</Group>
            )}
            {!projectShut && !taskShut && <>
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
            </>}
          </Fragment>
        )
      })}
      </>}
    </Panel>
    </div>
  )
}
