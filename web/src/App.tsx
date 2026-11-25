import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react"
import { bindVisualViewport } from '@/lib/touchViewport'
import { ArrowLeft, Bot, ChevronDown, ChevronUp, Kanban, Maximize2, Minimize2, Monitor, Moon, MoreHorizontal, PanelLeftClose, PanelLeftOpen, Plus, Settings, SquareTerminal, Sun, Workflow } from "lucide-react"
import { nextTheme, useWebTheme } from "@/lib/theme"
import { SettingsDialog } from "@/components/SettingsDialog"
import { api } from "@/api"
import { AgentView } from "@/components/AgentView"
import { Notifications } from '@/components/Notifications'
import { agentLabel } from '@/lib/status'
import { notifications, nextNotice, useAlerts, type Notice } from '@/lib/notifications'
import { Board, TaskDrawer } from "@/components/Board"
import { BoardConfigPage } from "@/components/BoardConfigPage"
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
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { useKeys } from "@/hooks/useKeys"
import { usePoll } from "@/hooks/usePoll"
import { useProjectDir } from "@/hooks/useProjectDir"
import { useRailCollapsed } from "@/hooks/useRail"
import { useRailLayout, type RailId } from "@/hooks/useRailLayout"
import { enc, nav, parseSel, useRoute } from "@/hooks/useRoute"
import { baseName, byTaskOrder } from "@/lib/status"
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
  const [settings, setSettings] = useState(false)
  // Rail collapsed state: an icon strip keeps the board room to itself.
  const [railCollapsed, setRailCollapsed] = useRailCollapsed()
  const layout = useRailLayout()
  const [editing, setEditing] = useState(false)
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

  const agent = sel?.kind === "agent" ? state?.agents.find((a) => a.pane === sel.pane) : undefined
  const [dir, setDir] = useProjectDir(agent?.path)
  // The rail lists only this workspace's agents; project is the main repo's name, so worktree agents count too.
  // An agent belongs to the deepest workspace holding its path, so a home-folder workspace doesn't swallow every project.
  const dirAgents = useMemo(() => {
    if (!dir) return ordered
    const projects = state?.projects ?? []
    const owner = (p: string) => projects.filter(d => p === d || p.startsWith(d + "/")).sort((a, b) => b.length - a.length)[0]
    return ordered.filter(a => { const o = owner(a.path); return o ? o === dir : a.project === baseName(dir) })
  }, [ordered, dir, state?.projects])
  // #/runs/<ticket>?dir=<project> names its project, so a shared link opens that project's run; without it the run uses the ambient dir.
  const runMatch = /^\/runs\/([^/?]+)(?:\?(.*))?$/.exec(route)
  // State loads async: a cold link counts its project only once the project list has it.
  const runParam = runMatch ? new URLSearchParams(runMatch[2] ?? "").get("dir") : null
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

  if (error?.message.startsWith("unauthorized")) {
    return <div className="app-viewport flex items-center justify-center p-6 text-center"><p>tagent: open the link printed by <code>tagent web</code> (it carries the access token).</p></div>
  }

  const proj = projPoll.data
  const boardRoute = route === '/board'
  const boardConfigRoute = route === '/board/config'
  const flowRoute = /^\/workflows(?:\/([^/]+))?$/.exec(route)
  // Old ticket links (/workflows/<wf>/<ticket>) open the run page, not the editor.
  const runRoute = runMatch ?? /^\/workflows\/[^/]+\/([^/]+)$/.exec(route)
  const agentsRoute = route === "/agents"
  const home = !sel && !boardRoute && !boardConfigRoute && !flowRoute && !runRoute && !agentsRoute
  const board = <section className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border bg-card"><Board dir={dir} proj={proj} reload={projPoll.reload} /></section>
  const tasks = <TasksPanel proj={proj} dir={dir} reload={projPoll.reload} className="min-h-0 flex-1" />


  const railIds = layout.order
  // Desktop rail never overflows: on a short window every panel shrinks and scrolls inside.
  const railCls: Record<RailId, string> = { overview: "max-h-[40%]", agents: "max-h-[35%]", tasks: "min-h-24 flex-1" }

  const railPanel = (id: RailId, i: number, ids: RailId[], cls: Record<RailId, string>) => <div key={id} className={cn("group/rail relative flex min-h-0 flex-col", id === "overview" && "overflow-y-auto", cls[id])} style={ids.every(x => layout.heights[x]) ? { flex: `${layout.heights[id]} 1 0%`, minHeight: 64, maxHeight: "none" } : undefined}>
              {editing && <div className="absolute top-1 left-1/2 z-10 flex -translate-x-1/2 rounded-md bg-card/90 shadow-sm ring-1 ring-border">
                <button aria-label="Move panel up" disabled={i === 0} className="rounded hover:bg-accent disabled:invisible" onClick={() => layout.move(id, -1, ids)}><ChevronUp className="size-4" /></button>
                <button aria-label="Move panel down" disabled={i === ids.length - 1} className="rounded hover:bg-accent disabled:invisible" onClick={() => layout.move(id, 1, ids)}><ChevronDown className="size-4" /></button>
              </div>}
              {id === "overview" && <OverviewPanel days={usageDays} onDays={setUsageDays} overview={overview} className="shrink-0" />}
              {id === "agents" && <AgentsPanel ordered={dirAgents} byTask={byTask} onToggleTask={toggleByTask} selPane={agent?.pane} className="min-h-0 flex-1" />}
              {id === "tasks" && tasks}
              {editing && i < ids.length - 1 && <div role="separator" aria-orientation="horizontal" title="Drag to resize (double-click to reset)" className="group/grip absolute inset-x-0 bottom-0 z-10 flex h-3 cursor-row-resize touch-none items-end justify-center"
                onDoubleClick={() => layout.setHeights({})}
                onPointerDown={(e) => {
                  // The bar trades height between this panel and the next; the panels always share the whole rail.
                  const panels = Array.from(e.currentTarget.parentElement!.parentElement!.children) as HTMLElement[]
                  e.currentTarget.setPointerCapture(e.pointerId)
                  const hs = panels.map(p => p.offsetHeight), total = hs.reduce((a, b) => a + b, 0) || 1
                  const pair = hs[i]! + hs[i + 1]!, h0 = hs[i]!, y0 = e.clientY
                  const apply = () => panels.forEach((p, k) => { p.style.flex = `${(hs[k]! / total) * 100} 1 0%`; p.style.maxHeight = "none" })
                  const h = (ev: PointerEvent) => { hs[i] = Math.max(64, Math.min(pair - 64, h0 + ev.clientY - y0)); hs[i + 1] = pair - hs[i]!; apply() }
                  const up = () => { window.removeEventListener("pointermove", h); window.removeEventListener("pointerup", up); layout.setHeights(Object.fromEntries(ids.map((x, k) => [x, Math.round((hs[k]! / total) * 1000) / 10]))) }
                  apply(); window.addEventListener("pointermove", h); window.addEventListener("pointerup", up)
                }}><span className="mb-0.5 h-1 w-10 rounded-full bg-muted-foreground/30 group-hover/grip:bg-brand" /></div>}
  </div>
  const phoneIds: RailId[] = layout.order
  // Phone home fits the screen like the desktop rail: no page scroll, each panel scrolls inside.
  const phoneCls: Record<RailId, string> = { overview: "max-h-[30%]", agents: "max-h-[30%]", tasks: "min-h-24 flex-1" }

  let main
  if (boardRoute) main = board
  else if (boardConfigRoute) main = <BoardConfigPage key={dir} dir={dir} />
  else if (runRoute) main = <RunPage key={(runDir ?? dir) + runRoute[1]} dir={runDir ?? dir} id={decodeURIComponent(runRoute[1]!)} />
  else if (flowRoute) main = <WorkflowsPage key={dir + route} dir={dir} initial={decodeURIComponent(flowRoute[1] ?? '')} />
  else if (agentsRoute) main = <AgentsPage key={dir} dir={dir} />
  else if (agent) main = <AgentView key={agent.pane} agent={agent} project={proj} />
  else if (sel) main = state ? <div className="p-6 text-muted-foreground">Agent not found. <a href="#/" className="text-foreground underline">Home</a></div> : null
  else main = <>
    {/* Phone home: the rail's lists stacked; the board is in the header. */}
    <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-hidden md:hidden">
      {phoneIds.map((id, i) => railPanel(id, i, phoneIds, phoneCls))}
    </div>
    <div className="hidden min-h-0 min-w-0 flex-1 md:flex">
      {board}
    </div>
  </>

  return (
    <TooltipProvider>
      <div className="app-viewport flex flex-col bg-sidebar">
        <header className="flex h-11 shrink-0 items-center gap-1 border-b bg-card px-2 md:gap-2 md:px-3">
          {!home && <Button variant="ghost" size="icon-xs" className={`max-sm:size-9 ${boardRoute ? "md:hidden" : ""}`} onClick={() => nav("/")} aria-label="Back to home" title="Back to home"><ArrowLeft /></Button>}
          <Button variant="ghost" size="icon-xs" className="hidden md:inline-flex" onClick={() => setRailCollapsed(!railCollapsed)} aria-label={railCollapsed ? "Show sidebar" : "Hide sidebar"} title={railCollapsed ? "Show sidebar" : "Hide sidebar"}>{railCollapsed ? <PanelLeftOpen /> : <PanelLeftClose />}</Button>
          <a href="#/" className="flex shrink-0 items-center gap-1.5 text-[13px] font-semibold"><span className="flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground"><SquareTerminal className="size-3.5" /></span><span className="hidden sm:inline">tagent</span><span className="hidden font-normal text-muted-foreground lg:inline"> · {state?.host ?? ""}</span></a>
          <div className="min-w-0 flex-1 overflow-hidden [&_button]:max-w-full"><ProjectSelect value={dir} projects={state?.projects ?? []} onChange={(d) => { setDir(d); nav("/") }} /></div>
          <div className="ml-auto flex items-center gap-1">
            <Button variant="ghost" size="icon-xs" className="max-sm:hidden" aria-label="Board" title="Board" onClick={() => nav('/board')}><Kanban /></Button>
            <Button variant="ghost" size="icon-xs" className="max-sm:hidden" aria-label="Agents" title="Agents" onClick={() => nav('/agents')}><Bot /></Button>
            <Button variant="ghost" size="icon-xs" className="max-sm:hidden" aria-label="Workflows" title="Workflows" onClick={() => nav('/workflows')}><Workflow /></Button>
            <Notifications items={notices} dir={dir ?? ''} onOpen={openNotice} />
            <Button size="sm" className="max-sm:size-9" onClick={() => setNewOpen(true)}><Plus /> <span className="hidden sm:inline">New agent</span></Button>
            {canFull && <Button className="hidden sm:inline-flex" variant="ghost" size="icon-xs" aria-label={full ? "Exit full screen" : "Full screen"} title={full ? "Exit full screen" : "Full screen"} onClick={() => void toggleFull()}>{full ? <Minimize2 /> : <Maximize2 />}</Button>}
            <Button variant="ghost" size="icon-xs" className="max-sm:hidden" aria-label={`Web theme: ${webTheme}`} title={`Web theme: ${webTheme} (this browser only)`} onClick={() => setWebTheme(nextTheme[webTheme])}>{webTheme === "dark" ? <Moon /> : webTheme === "light" ? <Sun /> : <Monitor />}</Button>
            <Button variant="ghost" size="icon-xs" className="max-sm:hidden" aria-label="Settings" title="Settings" onClick={() => setSettings(true)}><Settings /></Button>
            {/* Phone: the header keeps back, project, new agent and notifications; the rest lives here. */}
            <DropdownMenu>
              <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-xs" className="max-sm:size-9 sm:hidden" aria-label="More" title="More"><MoreHorizontal /></Button></DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => nav('/board')}>Board</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => nav('/agents')}>Agents</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => nav('/workflows')}>Workflows</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setWebTheme(nextTheme[webTheme])}>Theme: {webTheme}</DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setSettings(true)}>Settings</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>

        {editing && <div className="flex shrink-0 items-center gap-2 border-b bg-card px-3 py-1.5 text-xs text-muted-foreground"><span className="min-w-0 flex-1">Editing layout: use ▲▼ to move, drag the bar under a panel to resize.</span><Button size="xs" onClick={() => setEditing(false)}>Done</Button></div>}
        <div className="flex min-h-0 flex-1 gap-2 p-2">
          {railCollapsed ? <RailStrip tasks onExpand={() => setRailCollapsed(false)} /> : <aside className="hidden w-72 shrink-0 flex-col gap-2 md:flex">
            {railIds.map((id, i) => railPanel(id, i, railIds, railCls))}
          </aside>}
          <main className="flex min-h-0 min-w-0 flex-1 flex-col">{main}</main>
        </div>
      </div>
      <TaskDrawer proj={proj} dir={dir} reload={projPoll.reload} id={noticeTicket?.dir === dir ? noticeTicket.id : undefined} initialReply={noticeTicket?.reply} origin={noticeOrigin} onClose={() => setNoticeTicket(undefined)} />
      {newOpen && <NewAgentDialog dir={dir} onOpenChange={setNewOpen} />}
      <SettingsDialog open={settings} onOpenChange={setSettings} onEditLayout={() => { setSettings(false); setEditing(true) }} />
      <Toaster richColors position="top-right" />
    </TooltipProvider>
  )
}
