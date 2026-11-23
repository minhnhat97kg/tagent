import { useMemo, useState, type ReactNode } from "react"
import { ArrowLeft, Plus } from "lucide-react"
import { api } from "@/api"
import { AgentView } from "@/components/AgentView"
import { HelpDialog } from "@/components/HelpDialog"
import { NewAgentComposer } from "@/components/NewAgentComposer"
import { NewAgentDialog } from "@/components/NewAgentDialog"
import { AgentsPanel } from "@/components/panels/AgentsPanel"
import { DefsPanel } from "@/components/panels/DefsPanel"
import { ChatPanel } from "@/components/panels/ChatPanel"
import { OverviewPanel } from "@/components/panels/OverviewPanel"
import { TasksPanel } from "@/components/panels/TasksPanel"
import { Button } from "@/components/ui/button"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { useKeys } from "@/hooks/useKeys"
import { useProjectDir } from "@/hooks/useProjectDir"
import { enc, nav, parseSel, useRoute } from "@/hooks/useRoute"
import { usePoll } from "@/hooks/usePoll"
import { baseName } from "@/lib/status"
import { cn } from "@/lib/utils"
import { useAppState } from "@/state"
import type { Agent, Overview, Project } from "@/types"

export default function App() {
  const route = useRoute()
  const sel = parseSel(route)
  const { state, error } = useAppState()
  const [newOpen, setNewOpen] = useState(false)
  const [help, setHelp] = useState(false)

  const agents = useMemo(() => (state?.agents ?? []).filter((a) => !a.hidden), [state])
  // Agents grouped by project, in the order the list shows them and j/k walk them.
  const ordered = useMemo(() => {
    const groups = new Map<string, Agent[]>()
    for (const a of agents) {
      const k = a.project || baseName(a.path)
      groups.set(k, [...(groups.get(k) ?? []), a])
    }
    return [...groups.values()].flat()
  }, [agents])

  const agent = sel?.kind === "agent" ? state?.agents.find((a) => a.pane === sel.pane) : undefined
  const [dir, setDir] = useProjectDir(agent?.path)
  const projPoll = usePoll<Project | undefined>(
    () => (dir ? api<Project>(`/api/project?dir=${enc(dir)}`) : Promise.resolve(undefined)),
    2000,
    dir,
  )
  const overviewPoll = usePoll<Overview | undefined>(() => dir ? api<Overview>(`/api/overview?dir=${enc(dir)}`) : Promise.resolve(undefined), 2000, dir)
  const overview = overviewPoll.data?.dir === dir ? overviewPoll.data : undefined
  useKeys(ordered, agent?.pane, () => setHelp(true))

  if (error?.message.startsWith("unauthorized")) {
    return (
      <div className="flex h-dvh items-center justify-center p-6 text-center">
        <p>
          tmux-agent: open the link printed by <code>tmux-agent web</code> (it carries the access token).
        </p>
      </div>
    )
  }

  let body: ReactNode
  if (agent) body = <AgentView key={agent.pane} agent={agent} />
  else if (sel) body = state ? <NotFound /> : null
  else body = <Empty />

  return (
    <TooltipProvider>
      <div className="flex h-dvh flex-col">
        <header className="flex h-11 shrink-0 bg-sidebar items-center gap-2 border-b px-3">
          {sel && (
            <Button variant="ghost" size="icon" className="md:hidden" onClick={() => nav("/")} aria-label="Back">
              <ArrowLeft />
            </Button>
          )}
          <span className="font-serif text-sm">
            tmux-agent <span className="text-muted-foreground">· {state?.host ?? ""}</span>
          </span>
          <span className="truncate rounded-full bg-muted px-2 py-1 text-xs">{overview ? `${overview.attention.needed ? "⚠" : "✓"} ${overview.attention.text}` : "Loading…"}</span>
          <div className="ml-auto flex items-center gap-1">
            <Button size="sm" onClick={() => setNewOpen(true)}>
              <Plus /> New agent
            </Button>
            <Button variant="ghost" size="icon" aria-label="Shortcuts" onClick={() => setHelp(true)}>
              ?
            </Button>
          </div>
        </header>

        <div className="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[clamp(360px,32%,440px)_1fr]">
          <aside
            className={cn(
              "h-full min-h-0 flex-col gap-2.5 border-r bg-sidebar p-2.5 overflow-y-auto",
              sel ? "hidden md:flex" : "flex",
            )}
          >
            <OverviewPanel
              className="shrink-0"
              overview={overview}
              projects={state?.projects ?? []}
              dir={dir}
              onDir={setDir}
            />
            <AgentsPanel className="min-h-28 flex-[3_1_0] max-md:min-h-40 max-md:flex-none" ordered={ordered} selPane={agent?.pane} />
            <DefsPanel
              className="min-h-24 flex-[2_1_0] max-md:flex-none"
              proj={projPoll.data}
              dir={dir}
            />
            <TasksPanel
              className="min-h-24 flex-[2_1_0] max-md:flex-none"
              proj={projPoll.data}
              dir={dir}
              reload={projPoll.reload}
            />
            <ChatPanel key={dir} dir={dir} onTasks={projPoll.reload} className="min-h-48 flex-[2_1_0] max-md:min-h-64 max-md:flex-none" />
          </aside>

          <main className={cn("min-h-0 min-w-0 flex-col", sel ? "flex" : "hidden md:flex")}>{body}</main>
        </div>
      </div>
      {newOpen && <NewAgentDialog onOpenChange={setNewOpen} />}
      <HelpDialog open={help} onOpenChange={setHelp} />
      <Toaster richColors position="top-right" />
    </TooltipProvider>
  )
}

function Empty() {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center overflow-y-auto p-4">
      <div className="flex w-full max-w-2xl flex-col gap-4">
        <h2 className="font-serif text-2xl">What should an agent do?</h2>
        <NewAgentComposer />
      </div>
    </div>
  )
}

function NotFound() {
  return (
    <div className="p-6">
      <p className="text-muted-foreground">Agent not found.</p>
      <a href="#/" className="text-primary underline">
        Home
      </a>
    </div>
  )
}
