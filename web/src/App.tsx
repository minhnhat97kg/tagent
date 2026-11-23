import { useMemo, useState } from "react"
import { ArrowLeft, Plus, SquareTerminal } from "lucide-react"
import { api } from "@/api"
import { AgentView } from "@/components/AgentView"
import { Board } from "@/components/Board"
import { HelpDialog } from "@/components/HelpDialog"
import { NewAgentDialog } from "@/components/NewAgentDialog"
import { ProjectSelect } from "@/components/ProjectSelect"
import { AgentsPanel } from "@/components/panels/AgentsPanel"
import { ChatPanel } from "@/components/panels/ChatPanel"
import { DefsPanel } from "@/components/panels/DefsPanel"
import { OverviewPanel } from "@/components/panels/OverviewPanel"
import { TasksPanel } from "@/components/panels/TasksPanel"
import { Button } from "@/components/ui/button"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { useKeys } from "@/hooks/useKeys"
import { usePoll } from "@/hooks/usePoll"
import { useProjectDir } from "@/hooks/useProjectDir"
import { enc, nav, parseSel, useRoute } from "@/hooks/useRoute"
import { baseName } from "@/lib/status"
import { cn } from "@/lib/utils"
import { useAppState } from "@/state"
import type { Agent, Overview, Project } from "@/types"

// App is a fixed shell: top bar, a left rail (usage, agents, roles, tasks) and one main view.
// Home shows the board (chat in the rail); /agent/<pane> shows that agent's terminal.
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
  const projPoll = usePoll<Project | undefined>(() => (dir ? api<Project>(`/api/project?dir=${enc(dir)}`) : Promise.resolve(undefined)), 2000, dir)
  const overviewPoll = usePoll<Overview | undefined>(() => (dir ? api<Overview>(`/api/overview?dir=${enc(dir)}`) : Promise.resolve(undefined)), 2000, dir)
  const overview = overviewPoll.data?.dir === dir ? overviewPoll.data : undefined
  useKeys(ordered, agent?.pane, () => setHelp(true))

  if (error?.message.startsWith("unauthorized")) {
    return <div className="flex h-dvh items-center justify-center p-6 text-center"><p>tagent: open the link printed by <code>tagent web</code> (it carries the access token).</p></div>
  }

  const proj = projPoll.data
  const chatRoute = route === "/chat"
  const home = !sel && !chatRoute
  const tasks = <TasksPanel proj={proj} dir={dir} reload={projPoll.reload} className="min-h-40 flex-1" />


  let main
  if (chatRoute) main = <ChatPanel key={dir} dir={dir} onTasks={projPoll.reload} className="h-full" />
  else if (agent) main = <AgentView key={agent.pane} agent={agent} />
  else if (sel) main = state ? <div className="p-6 text-muted-foreground">Agent not found. <a href="#/" className="text-foreground underline">Home</a></div> : null
  else main = <>
    {/* Phone home: the rail's lists stacked; the board is one tap away. */}
    <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto md:hidden">
      <AgentsPanel ordered={ordered} chatActive={chatRoute} className="shrink-0" />
      {tasks}
    </div>
    <div className="hidden min-h-0 min-w-0 flex-1 md:flex">
      <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border bg-card"><Board dir={dir} proj={proj} reload={projPoll.reload} /></section>
    </div>
  </>

  return (
    <TooltipProvider>
      <div className="flex h-dvh flex-col bg-sidebar">
        <header className="flex h-11 shrink-0 items-center gap-2 border-b bg-card px-3">
          {!home && <Button variant="ghost" size="icon-xs" className="md:hidden" onClick={() => nav("/")} aria-label="Back"><ArrowLeft /></Button>}
          <a href="#/" className="flex shrink-0 items-center gap-1.5 text-[13px] font-semibold"><span className="flex size-6 items-center justify-center rounded-md bg-zinc-900 text-white"><SquareTerminal className="size-3.5" /></span>tagent<span className="hidden font-normal text-muted-foreground lg:inline"> · {state?.host ?? ""}</span></a>
          <ProjectSelect value={dir} projects={state?.projects ?? []} onChange={(d) => { setDir(d); nav("/") }} />
          {overview && <span className={cn("hidden min-w-0 items-center gap-1.5 truncate rounded-full px-2 py-0.5 text-[11.5px] md:flex", overview.attention.needed ? "bg-amber-50 text-amber-800" : "text-muted-foreground")}>
            <span className={cn("size-1.5 shrink-0 rounded-full", overview.attention.needed ? "bg-amber-500" : "bg-emerald-500")} />{overview.attention.text}
          </span>}
          <div className="ml-auto flex items-center gap-1">
            <Button size="sm" onClick={() => setNewOpen(true)}><Plus /> <span className="hidden sm:inline">New agent</span></Button>
            <Button variant="ghost" size="icon-xs" aria-label="Shortcuts" onClick={() => setHelp(true)}>?</Button>
          </div>
        </header>

        <div className="flex min-h-0 flex-1 gap-2 p-2">
          <aside className="hidden w-72 shrink-0 flex-col gap-2 md:flex">
            <OverviewPanel overview={overview} className="shrink-0" />
            <AgentsPanel ordered={ordered} selPane={agent?.pane} chatActive={chatRoute} className={home ? "min-h-24 flex-1" : "max-h-[35%] shrink-0"} />
            <DefsPanel proj={proj} dir={dir} className="max-h-[25%] shrink-0" />
            {!home && tasks}
          </aside>
          <main className="flex min-h-0 min-w-0 flex-1 flex-col">{main}</main>
        </div>
      </div>
      {newOpen && <NewAgentDialog onOpenChange={setNewOpen} />}
      <HelpDialog open={help} onOpenChange={setHelp} />
      <Toaster richColors position="top-right" />
    </TooltipProvider>
  )
}
