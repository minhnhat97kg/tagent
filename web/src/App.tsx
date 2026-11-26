import { useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react"
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
import { CustomPage } from "@/components/CustomPage"
import { OverviewPanel } from "@/components/panels/OverviewPanel"
import { Panel } from "@/components/Panel"
import { PaneChromeContext } from "@/lib/paneChrome"
import { TasksColumn } from "@/components/panels/TasksPane"
import { HomeLayout, type PhoneTab } from "@/components/HomeLayout"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { useKeys } from "@/hooks/useKeys"
import { usePoll } from "@/hooks/usePoll"
import { useProjectDir } from "@/hooks/useProjectDir"
import { enc, nav, parseRoute, useRoute } from "@/hooks/useRoute"
import { baseName, byTaskOrder } from "@/lib/status"
import { CustomPane } from "@/components/CustomPane"
import { PaneMenu } from "@/components/PaneMenu"
import type { ExtInfo } from "@/lib/ext"
import { addPane, DEFAULT_LAYOUT, extId, extName, loadPanes, panesOf, prune, removePane, savePanes, type Layout } from "@/lib/paneLayout"
import { toast } from "sonner"
import { useAppState } from "@/state"
import type { Agent, Overview, Project } from "@/types"

// ChatEmpty is the chat pane without an agent. Where a pane has a header to drag or maximize, it has one too.
function ChatEmpty({ className, children }: { className: string; children: ReactNode }) {
  if (!useContext(PaneChromeContext)) return <div className={className}>{children}</div>
  return <Panel className="min-h-0 flex-1" title="Chat" bodyClass="flex items-center justify-center p-6 text-sm text-muted-foreground"><p>{children}</p></Panel>
}

// App is a fixed shell: a top bar and one main view. Home is one page (agents, usage, chat, tasks);
// a maximized pane (#/pane/<id>) fills the view.
export default function App() {
  useLayoutEffect(() => bindVisualViewport(document.documentElement), [])
  const rawRoute = useRoute()
  const route = parseRoute(rawRoute)
  const { state, error } = useAppState()
  const [newOpen, setNewOpen] = useState(false)
  const [settings, setSettings] = useState(false)
  const [addWorkspace, setAddWorkspace] = useState(false)
  // The home layout: this device's saved columns and sizes, or undefined for the default.
  const [layout, setLayout] = useState(loadPanes)
  const changePanes = (l: Layout) => { setLayout(l); savePanes(l) }
  const resetPanes = () => { setLayout(undefined); savePanes(undefined); setSettings(false) }
  // The agent in the chat pane: picked in place, or named by an old link (#/agent/<pane>).
  const [picked, setPicked] = useState<string>()
  const [phoneTab, setPhoneTab] = useState<PhoneTab>("agents")
  // Custom panes (<config dir>/ui/*.html); the header and More menu show "Custom" only when there are some.
  const extPoll = usePoll<ExtInfo[]>(() => api<ExtInfo[]>("/api/ext").then(l => (Array.isArray(l) ? l : [])), 5000)
  const exts = extPoll.data
  const panes = useMemo(() => (exts ?? []).map(e => e.name), [exts])
  // The layout without custom panes that no longer exist (a saved one may name a deleted pane); kept as saved until the list is known.
  const shown = useMemo(() => {
    const now = layout ?? DEFAULT_LAYOUT
    return (exts && prune(now, id => { const n = extName(id); return !n || panes.includes(n) })) || now
  }, [layout, exts, panes])
  const addToLayout = (id: string) => changePanes(addPane(shown, id))
  const removeFromLayout = (id: string) => { const next = removePane(shown, id); if (next) changePanes(next) }
  // A pane an agent adds while the app is open is offered, never added.
  const known = useRef<Set<string>>(undefined)
  const addRef = useRef(addToLayout)
  addRef.current = addToLayout
  useEffect(() => {
    if (!exts) return
    for (const e of exts) if (known.current && !known.current.has(e.name) && e.agent) {
      toast(`Agent added pane ${e.name}`, { duration: 20000, action: { label: "Add", onClick: () => addRef.current(extId(e.name)) } })
    }
    known.current = new Set(exts.map(e => e.name))
  }, [exts])
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

  const home = route.page === "home"
  const routeAgent = route.page === "home" ? route.agent : undefined
  // Selecting an agent changes the chat pane in place; from another page it goes home first.
  const select = (pane: string) => {
    setPicked(pane)
    setPhoneTab("chat")
    if (rawRoute !== "/" && rawRoute !== "/pane/chat") nav("/")
  }
  // A cold link (#/agent/<pane>) opens on the chat tab of a phone too; the route itself stays.
  useEffect(() => { if (routeAgent) { setPicked(routeAgent); setPhoneTab("chat") } }, [routeAgent])
  const selPane = routeAgent ?? picked
  const agent = home || (route.page === "pane" && route.pane === "chat") ? state?.agents.find((a) => a.pane === selPane) : undefined
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
    if ('agent' in n) select(n.agent.pane)
    else if (n.kind === 'config') nav('/workflows')
    else {
      noticeOrigin.current = document.activeElement as HTMLElement
      setNoticeTicket({dir, id:n.task.id, reply:reply && n.role ? `@${n.role} ` : ''})
    }
  }
  useKeys(ordered, agent?.pane, select, () => {
    const next = nextNotice(notices, lastNotice.current ?? (agent ? 'agent:' + agent.pane : undefined))
    if (next) openNotice(next)
  })
  // The Board is the Tasks pane now: an old #/board link lands on home.
  useEffect(() => { if (rawRoute === "/board") location.replace("#/") }, [rawRoute])

  if (error?.message.startsWith("unauthorized")) {
    return <div className="app-viewport flex items-center justify-center p-6 text-center"><p>tagent: open the link printed by <code>tagent web</code> (it carries the access token).</p></div>
  }

  const proj = projPoll.data
  // A maximized pane fills the view. The Agents definitions page keeps the agent list beside its content.
  const minimizable = (id: string, node: ReactNode) => <PaneChromeContext.Provider value={{ id, maximized: true }}>{node}</PaneChromeContext.Provider>
  const maxed = (pane: string, node: ReactNode) => <div data-max={pane} className="flex min-h-0 min-w-0 flex-1 flex-col">{minimizable(pane, node)}</div>
  const withAgents = (content: ReactNode) => <div data-max="agents" className="flex min-h-0 min-w-0 flex-1 gap-2">
    <AgentsPanel className="w-64 shrink-0 max-lg:hidden" ordered={dirAgents} byTask={byTask} onToggleTask={toggleByTask} onSelect={select} selPane={agent?.pane} />
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">{content}</div>
  </div>
  const usagePane = <OverviewPanel className="min-h-0 flex-1 overflow-y-auto" days={usageDays} onDays={setUsageDays} overview={overview} />
  const agentsPane = <AgentsPanel className="min-h-0 flex-1" ordered={dirAgents} byTask={byTask} onToggleTask={toggleByTask} onSelect={select} selPane={agent?.pane} />
  const tasksPane = <TasksColumn proj={proj} dir={dir} reload={projPoll.reload} />
  const chatPane = agent ? <AgentView key={agent.pane} agent={agent} project={proj} />
    : routeAgent ? state ? <ChatEmpty className="p-6 text-muted-foreground">Agent not found. <a href="#/" className="text-foreground underline">Home</a></ChatEmpty> : null
    : <ChatEmpty className="flex min-h-0 flex-1 items-center justify-center rounded-lg border bg-card text-sm text-muted-foreground">Pick an agent</ChatEmpty>

  let main
  const extPanes = Object.fromEntries((exts ?? []).map(e => [extId(e.name), <CustomPane key={e.name} info={e} reload={() => void extPoll.reload()} />]))
  if (home) main = <HomeLayout saved={shown} onChange={changePanes} onRemove={removeFromLayout} compact={state?.compact} tab={phoneTab} onTab={setPhoneTab}
    panes={{ agents: agentsPane, usage: usagePane, chat: chatPane, tasks: tasksPane, ...extPanes }} />
  else if (route.page === "pane") main = maxed(route.pane, extName(route.pane) ? extPanes[route.pane] ?? <div className="p-6 text-sm text-muted-foreground">{exts ? "No custom pane by that name." : null}</div> : route.pane === "usage" ? usagePane : route.pane === "chat" ? chatPane : tasksPane)
  else if (route.page === "agents") main = withAgents(minimizable("agents", <AgentsPage key={dir} dir={dir} />))
  else if (route.page === "flows") main = maxed("workflows", <WorkflowsPage key={dir + rawRoute} dir={dir} initial={route.name} />)
  else if (route.page === "run") main = maxed("workflows", <RunPage key={(runDir ?? dir) + route.ticket} dir={runDir ?? dir} id={route.ticket} />)
  else if (route.page === "boardConfig") main = maxed("workflows", <BoardConfigPage key={dir} dir={dir} />)
  else if (route.page === "routing") main = <RoutingPage key={dir} dir={dir} />
  else if (route.page === "custom") main = <CustomPage names={panes} name={route.name} />
  else main = null

  return (
    <TooltipProvider>
      <div className="app-viewport flex flex-col bg-sidebar">
        {/* viewport-fit=cover lets the page reach under the status bar and the window overlay; the bar's insets keep its content clear of them. */}
        <header className="flex h-[calc(2.75rem+env(safe-area-inset-top))] shrink-0 items-center gap-1 border-b bg-card pt-[env(safe-area-inset-top)] pl-[max(0.5rem,env(safe-area-inset-left))] pr-[max(0.5rem,env(safe-area-inset-right))] md:gap-2 md:pl-[max(0.75rem,env(safe-area-inset-left))] md:pr-[max(0.75rem,env(safe-area-inset-right))]">
          {!home && <Button variant="ghost" size="icon-xs" className="max-sm:size-9" onClick={() => nav("/")} aria-label="Back to home" title="Back to home"><ArrowLeft /></Button>}
          <a href="#/" className="flex shrink-0 items-center gap-1.5 text-[13px] font-semibold"><span className="flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground"><SquareTerminal className="size-3.5" /></span><span className="hidden sm:inline">tagent</span><span className="hidden font-normal text-muted-foreground lg:inline"> · {state?.host ?? ""}</span></a>
          <div className="min-w-0 flex-1 overflow-hidden [&_button]:max-w-full"><ProjectSelect value={dir} projects={state?.projects ?? []} onChange={(d) => { setDir(d); setPicked(undefined); nav("/") }} onAdd={() => setAddWorkspace(true)} /></div>
          <div className="ml-auto flex items-center gap-1">
            {panes.length > 0 && <a href="#/custom" className="max-md:hidden px-1.5 text-[13px] text-muted-foreground hover:text-foreground">Custom</a>}
            <PaneMenu shown={panesOf(shown)} exts={exts ?? []} onAdd={addToLayout} onChanged={extPoll.reload} />
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
                {panes.length > 0 && <DropdownMenuItem onSelect={() => nav('/custom')}>Custom</DropdownMenuItem>}
                <DropdownMenuItem onSelect={() => setWebTheme(nextTheme[webTheme])}>Theme: {webTheme}</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setSettings(true)}>Settings</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>

        <div className={`flex min-h-0 flex-1 ${home && state?.compact ? "" : "p-2"}`}>
          <main className="flex min-h-0 min-w-0 flex-1 flex-col">{main}</main>
        </div>
      </div>
      <TaskDrawer proj={proj} dir={dir} reload={projPoll.reload} id={noticeTicket?.dir === dir ? noticeTicket.id : undefined} initialReply={noticeTicket?.reply} origin={noticeOrigin} onClose={() => setNoticeTicket(undefined)} />
      {newOpen && <NewAgentDialog dir={dir} onOpenChange={setNewOpen} />}
      <SettingsDialog open={settings} onOpenChange={setSettings} onResetLayout={resetPanes} onAddWorkspace={() => { setSettings(false); setAddWorkspace(true) }} />
      <AddWorkspaceDialog open={addWorkspace} onOpenChange={setAddWorkspace} onAdded={(d) => { setDir(d); setPicked(undefined); nav("/") }} />
      <Toaster richColors position="top-right" />
    </TooltipProvider>
  )
}
