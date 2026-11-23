import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { bindVisualViewport } from '@/lib/touchViewport'
import { ArrowLeft, Bot, Kanban, Maximize2, Minimize2, PanelLeftClose, PanelLeftOpen, Plus, Settings, SquareTerminal, Workflow } from "lucide-react"
import { SettingsDialog } from "@/components/SettingsDialog"
import { api } from "@/api"
import { AgentView } from "@/components/AgentView"
import { Notifications } from '@/components/Notifications'
import { notifications, nextNotice, type Notice } from '@/lib/notifications'
import { Board, TaskDrawer } from "@/components/Board"
import { HelpDialog } from "@/components/HelpDialog"
import { NewAgentDialog } from "@/components/NewAgentDialog"
import { ProjectSelect } from "@/components/ProjectSelect"
import { RailStrip } from "@/components/Rail"
import { AgentsPanel } from "@/components/panels/AgentsPanel"
import { WorkflowsPage } from "@/components/WorkflowsPage"
import { RunPage } from "@/components/RunPage"
import { AgentsPage } from "@/components/AgentsPage"
import { OverviewPanel } from "@/components/panels/OverviewPanel"
import { TasksPanel } from "@/components/panels/TasksPanel"
import { Button } from "@/components/ui/button"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { useKeys } from "@/hooks/useKeys"
import { usePoll } from "@/hooks/usePoll"
import { useProjectDir } from "@/hooks/useProjectDir"
import { useRailCollapsed } from "@/hooks/useRail"
import { enc, nav, parseSel, useRoute } from "@/hooks/useRoute"
import { baseName } from "@/lib/status"
import { useAppState } from "@/state"
import type { Agent, Overview, Project } from "@/types"

// App is a fixed shell: top bar, a left rail (usage, agents, tasks) and one main view.
// The rail collapses to an icon strip (remembered per device). Home shows the
// board; /agent/<pane> shows that agent's terminal.
export default function App() {
  useLayoutEffect(() => bindVisualViewport(document.documentElement), [])
  const route = useRoute()
  const sel = parseSel(route)
  const { state, error } = useAppState()
  const [newOpen, setNewOpen] = useState(false)
  const [help, setHelp] = useState(false)
  const [settings, setSettings] = useState(false)
  // Rail collapsed state: an icon strip keeps the board room to itself.
  const [railCollapsed, setRailCollapsed] = useRailCollapsed()
  // Whole app full screen (browser chrome hidden). iPhone Safari has no Fullscreen API, so the button hides there.
  const [full, setFull] = useState(false)
  useEffect(() => {
    const sync = () => setFull(!!document.fullscreenElement)
    document.addEventListener("fullscreenchange", sync)
    return () => document.removeEventListener("fullscreenchange", sync)
  }, [])
  useEffect(() => { document.documentElement.classList.toggle("dark", state?.theme === "dark") }, [state?.theme])
  const canFull = typeof document.documentElement.requestFullscreen === "function"
  const toggleFull = () => (document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen()).catch(() => {})

  // Background agents (tagent-bg session) are listed too, after the visible ones, like the TUI.
  const agents = useMemo(() => [...(state?.agents ?? [])].sort((a, b) => Number(!!a.hidden) - Number(!!b.hidden)), [state])
  // Agents grouped by project, in the order the list shows them and j/k walk them.
  const ordered = useMemo(() => {
    const groups = new Map<string, Agent[]>()
    for (const a of agents) {
      const k = a.project || baseName(a.path)
      groups.set(k, [...(groups.get(k) ?? []), a])
    }
    return [...groups.values()].flat().sort((a, b) => Number(!!b.pinned) - Number(!!a.pinned))
  }, [agents])

  const agent = sel?.kind === "agent" ? state?.agents.find((a) => a.pane === sel.pane) : undefined
  const [dir, setDir] = useProjectDir(agent?.path)
  const projPoll = usePoll<Project | undefined>(() => (dir ? api<Project>(`/api/project?dir=${enc(dir)}`) : Promise.resolve(undefined)), 2000, dir)
  const [usageDays, setUsageDays] = useState(7)
  const overviewPoll = usePoll<Overview | undefined>(() => (dir ? api<Overview>(`/api/overview?days=${usageDays}&dir=${enc(dir)}`) : Promise.resolve(undefined)), 5000, dir + usageDays) // usage changes slowly; 5s keeps the server idle
  const overview = overviewPoll.data?.dir === dir ? overviewPoll.data : undefined
  const notices = notifications(ordered, projPoll.data?.dir === dir ? projPoll.data : undefined, overview)
  const [noticeTicket, setNoticeTicket] = useState<{id: string; reply: string; dir: string}>()
  const noticeOrigin = useRef<HTMLElement | null>(null)
  const lastNotice = useRef<string | undefined>(undefined)
  useEffect(() => { if (agent?.pane) lastNotice.current = 'agent:' + agent.pane }, [agent?.pane])
  const openNotice = (n: Notice, reply = false) => {
    lastNotice.current = n.id
    if ('agent' in n) nav('/agent/' + enc(n.agent.pane))
    else {
      noticeOrigin.current = document.activeElement as HTMLElement
      setNoticeTicket({dir, id:n.task.id, reply:reply && n.role ? `@${n.role} ` : ''})
    }
  }
  useKeys(ordered, agent?.pane, () => setHelp(true), () => {
    const next = nextNotice(notices, lastNotice.current ?? (agent ? 'agent:' + agent.pane : undefined))
    if (next) openNotice(next)
  })

  if (error?.message.startsWith("unauthorized")) {
    return <div className="app-viewport flex items-center justify-center p-6 text-center"><p>tagent: open the link printed by <code>tagent web</code> (it carries the access token).</p></div>
  }

  const proj = projPoll.data
  const boardRoute = route === '/board'
  const flowRoute = /^\/workflows(?:\/([^/]+))?$/.exec(route)
  // Old ticket links (/workflows/<wf>/<ticket>) open the run page, not the editor.
  const runRoute = /^\/runs\/([^/]+)$/.exec(route) ?? /^\/workflows\/[^/]+\/([^/]+)$/.exec(route)
  const agentsRoute = route === "/agents"
  const home = !sel && !boardRoute && !flowRoute && !runRoute && !agentsRoute
  const board = <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border bg-card"><Board dir={dir} proj={proj} reload={projPoll.reload} /></section>
  const tasks = <TasksPanel proj={proj} dir={dir} reload={projPoll.reload} className="min-h-40 flex-1" />


  let main
  if (boardRoute) main = board
  else if (runRoute) main = <RunPage key={dir + runRoute[1]} dir={dir} id={decodeURIComponent(runRoute[1]!)} />
  else if (flowRoute) main = <WorkflowsPage key={dir + route} dir={dir} initial={decodeURIComponent(flowRoute[1] ?? '')} />
  else if (agentsRoute) main = <AgentsPage key={dir} dir={dir} />
  else if (agent) main = <AgentView key={agent.pane} agent={agent} project={proj} />
  else if (sel) main = state ? <div className="p-6 text-muted-foreground">Agent not found. <a href="#/" className="text-foreground underline">Home</a></div> : null
  else main = <>
    {/* Phone home: the rail's lists stacked; the board is in the header. */}
    <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto md:hidden">
      <OverviewPanel days={usageDays} onDays={setUsageDays} overview={overview} className="shrink-0" />
      <AgentsPanel ordered={ordered} className="shrink-0" />
      {tasks}
    </div>
    <div className="hidden min-h-0 min-w-0 flex-1 md:flex">
      {board}
    </div>
  </>

  return (
    <TooltipProvider>
      <div className="app-viewport flex flex-col bg-sidebar">
        <header className="flex h-11 shrink-0 items-center gap-1 border-b bg-card px-2 md:gap-2 md:px-3">
          {!home && <Button variant="ghost" size="icon-xs" className={boardRoute ? "md:hidden" : ""} onClick={() => nav("/")} aria-label="Back to home" title="Back to home"><ArrowLeft /></Button>}
          <Button variant="ghost" size="icon-xs" className="hidden md:inline-flex" onClick={() => setRailCollapsed(!railCollapsed)} aria-label={railCollapsed ? "Show sidebar" : "Hide sidebar"} title={railCollapsed ? "Show sidebar" : "Hide sidebar"}>{railCollapsed ? <PanelLeftOpen /> : <PanelLeftClose />}</Button>
          <a href="#/" className="flex shrink-0 items-center gap-1.5 text-[13px] font-semibold"><span className="flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground"><SquareTerminal className="size-3.5" /></span><span className="hidden sm:inline">tagent</span><span className="hidden font-normal text-muted-foreground lg:inline"> · {state?.host ?? ""}</span></a>
          <div className="min-w-0 flex-1 overflow-hidden [&_button]:max-w-full"><ProjectSelect value={dir} projects={state?.projects ?? []} onChange={(d) => { setDir(d); nav("/") }} /></div>
          <div className="ml-auto flex items-center gap-1">
            <Button variant="ghost" size="icon-xs" aria-label="Board" title="Board" onClick={() => nav('/board')}><Kanban /></Button>
            <Button variant="ghost" size="icon-xs" aria-label="Agents" title="Agents" onClick={() => nav('/agents')}><Bot /></Button>
            <Button variant="ghost" size="icon-xs" aria-label="Workflows" title="Workflows" onClick={() => nav('/workflows')}><Workflow /></Button>
            <Notifications items={notices} onOpen={openNotice} onReply={n => openNotice(n, true)} />
            <Button size="sm" onClick={() => setNewOpen(true)}><Plus /> <span className="hidden sm:inline">New agent</span></Button>
            {canFull && <Button className="hidden sm:inline-flex" variant="ghost" size="icon-xs" aria-label={full ? "Exit full screen" : "Full screen"} title={full ? "Exit full screen" : "Full screen"} onClick={() => void toggleFull()}>{full ? <Minimize2 /> : <Maximize2 />}</Button>}
            <Button variant="ghost" size="icon-xs" aria-label="Settings" title="Settings" onClick={() => setSettings(true)}><Settings /></Button>
            <Button variant="ghost" size="icon-xs" aria-label="Shortcuts" onClick={() => setHelp(true)}>?</Button>
          </div>
        </header>

        <div className="flex min-h-0 flex-1 gap-2 p-2">
          {railCollapsed ? <RailStrip tasks={!home} onExpand={() => setRailCollapsed(false)} /> : <aside className="hidden w-72 shrink-0 flex-col gap-2 md:flex">
            <OverviewPanel days={usageDays} onDays={setUsageDays} overview={overview} className="shrink-0" />
            <AgentsPanel ordered={ordered} selPane={agent?.pane} className={home ? "min-h-24 flex-1" : "max-h-[35%] shrink-0"} />
            {!home && tasks}
          </aside>}
          <main className="flex min-h-0 min-w-0 flex-1 flex-col">{main}</main>
        </div>
      </div>
      <TaskDrawer proj={proj} dir={dir} reload={projPoll.reload} id={noticeTicket?.dir === dir ? noticeTicket.id : undefined} initialReply={noticeTicket?.reply} origin={noticeOrigin} onClose={() => setNoticeTicket(undefined)} />
      {newOpen && <NewAgentDialog dir={dir} onOpenChange={setNewOpen} />}
      <SettingsDialog open={settings} onOpenChange={setSettings} />
      <HelpDialog open={help} onOpenChange={setHelp} />
      <Toaster richColors position="top-right" />
    </TooltipProvider>
  )
}
