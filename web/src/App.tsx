import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { bindVisualViewport } from '@/lib/touchViewport'
import { ArrowLeft, Maximize2, Minimize2, Monitor, Moon, MoreHorizontal, Plus, Settings, SquareTerminal, Sun } from "lucide-react"
import { nextTheme, useWebTheme } from "@/lib/theme"
import { SettingsDialog } from "@/components/SettingsDialog"
import { api } from "@/api"
import { AgentView } from "@/components/AgentView"
import { Notifications } from '@/components/Notifications'
import { agentLabel } from '@/lib/status'
import { notifications, nextNotice, useAlerts, type Notice } from '@/lib/notifications'
import { TaskDrawer } from "@/components/Board"
import { BoardConfigPage } from "@/components/BoardConfigPage"
import { NewAgentDialog } from "@/components/NewAgentDialog"
import { ProjectSelect } from "@/components/ProjectSelect"
import { AddWorkspaceDialog } from "@/components/AddWorkspaceDialog"
import { AgentsPanel } from "@/components/panels/AgentsPanel"
import { WorkflowsPage } from "@/components/WorkflowsPage"
import { RunPage } from "@/components/RunPage"
import { AgentsPage } from "@/components/AgentsPage"
import { RoutingPage } from "@/components/RoutingPage"
import { OverviewPanel } from "@/components/panels/OverviewPanel"
import { TasksPane } from "@/components/panels/TasksPane"
import { WorkflowsPane } from "@/components/panels/WorkflowsPane"
import { PaneGrid } from "@/components/PaneGrid"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { useKeys } from "@/hooks/useKeys"
import { usePoll } from "@/hooks/usePoll"
import { useProjectDir } from "@/hooks/useProjectDir"
import { enc, nav, parseRoute, useRoute } from "@/hooks/useRoute"
import { baseName, byTaskOrder } from "@/lib/status"
import { loadPanes, savePanes, type PaneBox } from "@/lib/paneLayout"
import { useAppState } from "@/state"
import type { Agent, Overview, Project } from "@/types"

// App is a fixed shell: a top bar and one main view. Home is the pane grid (usage,
// agents, workflows, tasks); a maximized pane (#/pane/<id>, or an old link such as /agent/<pane>) fills the view.
export default function App() {
  useLayoutEffect(() => bindVisualViewport(document.documentElement), [])
  const rawRoute = useRoute()
  const route = parseRoute(rawRoute)
  const { state, error } = useAppState()
  const [newOpen, setNewOpen] = useState(false)
  const [settings, setSettings] = useState(false)
  const [addWorkspace, setAddWorkspace] = useState(false)
  // The home layout: this device's saved boxes, or undefined for the default.
  const [panes, setPanes] = useState(loadPanes)
  const changePanes = (boxes: PaneBox[]) => { setPanes(boxes); savePanes(boxes) }
  const resetPanes = () => { setPanes(undefined); savePanes(undefined); setSettings(false) }
  // Whole app full screen (browser chrome hidden). iPhone Safari has no Fullscreen API, so the button hides there.
  const [full, setFull] = useState(false)
  useEffect(() => {
    const sync = () => setFull(!!document.fullscreenElement)
    document.addEventListener("fullscreenchange", sync)
    return () => document.removeEventListener("fullscreenchange", sync)
  }, [])
  const [webTheme, setWebTheme] = useWebTheme()
  const canFull = typeof document.documentElement.requestFullscreen === "function"
  const toggleFull = () => (document.fullscreenElement ? document.exitFullscreen() : document.documentElement.requestFullscreen()).catch(() => {})

  // Agents in the old tagent-bg session are listed too, after the visible ones, like the TUI.
  const agents = useMemo(() => [...(state?.agents ?? [])].sort((a, b) => Number(!!a.hidden) - Number(!!b.hidden)), [state])
  // Agents grouped by project, in the order the list shows them and j/k walk them.
  const [byTask, setByTask] = useState(() => { try { return localStorage.getItem("tagent.byTask") === "1" } catch { return false } })
  const toggleByTask = () => setByTask(v => { try { localStorage.setItem("tagent.byTask", v ? "0" : "1") } catch { /* not remembered in private mode */ } return !v })
  const ordered = useMemo(() => {
    const groups = new Map<string, Agent[]>()
    for (const a of agents) {
      const k = a.project || baseName(a.path)
      groups.set(k, [...(groups.get(k) ?? []), a])
    }
    const lists = [...groups.values()].map(g => byTask ? g.sort(byTaskOrder) : g)
    return lists.flat().sort((a, b) => Number(!!b.pinned) - Number(!!a.pinned))
  }, [agents, byTask])

  const agent = route.page === "agents" && route.agent ? state?.agents.find((a) => a.pane === route.agent) : undefined
  const [dir, setDir] = useProjectDir(agent?.path)
  // The agents list shows only this workspace's agents; project is the main repo's name, so worktree agents count too.
  // An agent belongs to the deepest workspace holding its path, so a home-folder workspace doesn't swallow every project.
  const dirAgents = useMemo(() => {
    if (!dir) return ordered
    const projects = state?.projects ?? []
    const owner = (p: string) => projects.filter(d => p === d || p.startsWith(d + "/")).sort((a, b) => b.length - a.length)[0]
    return ordered.filter(a => { const o = owner(a.path); return o ? o === dir : a.project === baseName(dir) })
  }, [ordered, dir, state?.projects])
  // #/runs/<ticket>?dir=<project> names its project, so a shared link opens that project's run; without it the run uses the ambient dir.
  // State loads async: a cold link counts its project only once the project list has it.
  const runParam = route.page === "run" ? route.dir : null
  const runDir = runParam && state?.projects?.includes(runParam) ? runParam : undefined
  useEffect(() => { if (runDir) setDir(runDir) }, [runDir])
  const projPoll = usePoll<Project | undefined>(() => (dir ? api<Project>(`/api/project?dir=${enc(dir)}`) : Promise.resolve(undefined)), 2000, dir)
  const [usageDays, setUsageDays] = useState(7)
  const overviewPoll = usePoll<Overview | undefined>(() => (dir ? api<Overview>(`/api/overview?days=${usageDays}&dir=${enc(dir)}`) : Promise.resolve(undefined)), 5000, dir + usageDays) // usage changes slowly; 5s keeps the server idle
  const overview = overviewPoll.data?.dir === dir ? overviewPoll.data : undefined
  const notices = notifications(ordered, projPoll.data?.dir === dir ? projPoll.data : undefined, overview)
  useAlerts(notices, n => 'agent' in n ? `${agentLabel(n.agent)}: ${n.agent.dialog?.question || n.agent.wait || n.agent.prompt}` : n.kind === 'config' ? n.message : n.task.title || n.reason)
  const [noticeTicket, setNoticeTicket] = useState<{id: string; reply: string; dir: string}>()
  const noticeOrigin = useRef<HTMLElement | null>(null)
  // A ticket card in chat asks for its drawer with a "tagent:ticket" event ({id, dir}).
  useEffect(() => {
    const f = (e: Event) => {
      const t = (e as CustomEvent<{ id: string; dir?: string }>).detail
      const d = t.dir || dir
      noticeOrigin.current = document.activeElement as HTMLElement
      if (d !== dir) setDir(d)
      setNoticeTicket({ dir: d, id: t.id, reply: '' })
    }
    window.addEventListener('tagent:ticket', f)
    return () => window.removeEventListener('tagent:ticket', f)
  }, [dir])
  const lastNotice = useRef<string | undefined>(undefined)
  useEffect(() => { if (agent?.pane) lastNotice.current = 'agent:' + agent.pane }, [agent?.pane])
  const openNotice = (n: Notice, reply = false) => {
    lastNotice.current = n.id
    if ('agent' in n) nav('/agent/' + enc(n.agent.pane))
    else if (n.kind === 'config') nav('/workflows')
    else {
      noticeOrigin.current = document.activeElement as HTMLElement
      setNoticeTicket({dir, id:n.task.id, reply:reply && n.role ? `@${n.role} ` : ''})
    }
  }
  useKeys(ordered, agent?.pane, () => {
    const next = nextNotice(notices, lastNotice.current ?? (agent ? 'agent:' + agent.pane : undefined))
    if (next) openNotice(next)
  })
  // The Board is the Tasks pane now: an old #/board link lands on home.
  useEffect(() => { if (rawRoute === "/board") location.replace("#/") }, [rawRoute])

  if (error?.message.startsWith("unauthorized")) {
    return <div className="app-viewport flex items-center justify-center p-6 text-center"><p>tagent: open the link printed by <code>tagent web</code> (it carries the access token).</p></div>
  }

  const proj = projPoll.data
  const home = route.page === "home"
  // A maximized pane fills the view. The Agents pane keeps the agent list beside its content.
  const maxed = (pane: string, node: ReactNode) => <div data-max={pane} className="flex min-h-0 min-w-0 flex-1 flex-col">{node}</div>
  const withAgents = (content: ReactNode) => <div data-max="agents" className="flex min-h-0 min-w-0 flex-1 gap-2">
    <AgentsPanel className="w-64 shrink-0 max-lg:hidden" ordered={dirAgents} byTask={byTask} onToggleTask={toggleByTask} selPane={agent?.pane} />
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">{content}</div>
  </div>
  const usagePane = <OverviewPanel className="min-h-0 flex-1" days={usageDays} onDays={setUsageDays} overview={overview} />
  const agentsPane = <AgentsPanel className="min-h-0 flex-1" ordered={dirAgents} byTask={byTask} onToggleTask={toggleByTask} selPane={agent?.pane} />
  const tasksPane = <TasksPane proj={proj} dir={dir} reload={projPoll.reload} />

  let main
  if (route.page === "home") main = <PaneGrid saved={panes} onChange={changePanes} compact={state?.compact} panes={{
    usage: usagePane, agents: agentsPane, workflows: <WorkflowsPane dir={dir} compact={state?.compact} />,
    tasks: <TasksPane proj={proj} dir={dir} reload={projPoll.reload} compact={state?.compact} />,
  }} />
  else if (route.page === "pane") main = maxed(route.pane, route.pane === "usage" ? usagePane : tasksPane)
  else if (route.page === "agents") main = withAgents(route.agent
    ? agent ? <AgentView key={agent.pane} agent={agent} project={proj} /> : state ? <div className="p-6 text-muted-foreground">Agent not found. <a href="#/" className="text-foreground underline">Home</a></div> : null
    : <AgentsPage key={dir} dir={dir} />)
  else if (route.page === "flows") main = maxed("workflows", <WorkflowsPage key={dir + rawRoute} dir={dir} initial={route.name} />)
  else if (route.page === "run") main = maxed("workflows", <RunPage key={(runDir ?? dir) + route.ticket} dir={runDir ?? dir} id={route.ticket} />)
  else if (route.page === "boardConfig") main = maxed("workflows", <BoardConfigPage key={dir} dir={dir} />)
  else if (route.page === "routing") main = <RoutingPage key={dir} dir={dir} />
  else main = null

  return (
    <TooltipProvider>
      <div className="app-viewport flex flex-col bg-sidebar">
        <header className="flex h-11 shrink-0 items-center gap-1 border-b bg-card px-2 md:gap-2 md:px-3">
          {!home && <Button variant="ghost" size="icon-xs" className="max-sm:size-9" onClick={() => nav("/")} aria-label="Back to home" title="Back to home"><ArrowLeft /></Button>}
          <a href="#/" className="flex shrink-0 items-center gap-1.5 text-[13px] font-semibold"><span className="flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground"><SquareTerminal className="size-3.5" /></span><span className="hidden sm:inline">tagent</span><span className="hidden font-normal text-muted-foreground lg:inline"> · {state?.host ?? ""}</span></a>
          <div className="min-w-0 flex-1 overflow-hidden [&_button]:max-w-full"><ProjectSelect value={dir} projects={state?.projects ?? []} onChange={(d) => { setDir(d); nav("/") }} onAdd={() => setAddWorkspace(true)} /></div>
          <div className="ml-auto flex items-center gap-1">
            <Notifications items={notices} dir={dir ?? ''} onOpen={openNotice} />
            <Button size="sm" className="max-sm:size-9" onClick={() => setNewOpen(true)}><Plus /> <span className="hidden sm:inline">New agent</span></Button>
            {canFull && <Button className="hidden sm:inline-flex" variant="ghost" size="icon-xs" aria-label={full ? "Exit full screen" : "Full screen"} title={full ? "Exit full screen" : "Full screen"} onClick={() => void toggleFull()}>{full ? <Minimize2 /> : <Maximize2 />}</Button>}
            <Button variant="ghost" size="icon-xs" className="max-sm:hidden" aria-label={`Web theme: ${webTheme}`} title={`Web theme: ${webTheme} (this browser only)`} onClick={() => setWebTheme(nextTheme[webTheme])}>{webTheme === "dark" ? <Moon /> : webTheme === "light" ? <Sun /> : <Monitor />}</Button>
            <Button variant="ghost" size="icon-xs" className="max-sm:hidden" aria-label="Settings" title="Settings" onClick={() => setSettings(true)}><Settings /></Button>
            {/* Below 768px (where the Workflows pane is not shown) the header keeps back, project, new agent and notifications; the rest lives here. */}
            <DropdownMenu>
              <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-xs" className="max-sm:size-9 md:hidden" aria-label="More" title="More"><MoreHorizontal /></Button></DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => nav('/agents')}>Agents</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => nav('/workflows')}>Workflows</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setWebTheme(nextTheme[webTheme])}>Theme: {webTheme}</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setSettings(true)}>Settings</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>

        <div className="flex min-h-0 flex-1 gap-2 p-2">
          <main className="flex min-h-0 min-w-0 flex-1 flex-col">{main}</main>
        </div>
      </div>
      <TaskDrawer proj={proj} dir={dir} reload={projPoll.reload} id={noticeTicket?.dir === dir ? noticeTicket.id : undefined} initialReply={noticeTicket?.reply} origin={noticeOrigin} onClose={() => setNoticeTicket(undefined)} />
      {newOpen && <NewAgentDialog dir={dir} onOpenChange={setNewOpen} />}
      <SettingsDialog open={settings} onOpenChange={setSettings} onResetLayout={resetPanes} onAddWorkspace={() => { setSettings(false); setAddWorkspace(true) }} />
      <AddWorkspaceDialog open={addWorkspace} onOpenChange={setAddWorkspace} onAdded={(d) => { setDir(d); nav("/") }} />
      <Toaster richColors position="top-right" />
    </TooltipProvider>
  )
}
