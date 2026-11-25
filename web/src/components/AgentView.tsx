import { act, post } from '@/api'
import { useRef, useState } from "react"
import { MoreHorizontal, ArrowRightLeft, MessageSquare, Terminal as TerminalIcon } from "lucide-react"
import type { TerminalControl, TerminalOwnership } from '@/lib/terminalFit'
import { Worktrees } from './Worktrees'
import { Panel } from "@/components/Panel"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { KeyBar } from "@/components/KeyBar"
import { DialogCard } from "@/components/DialogCard"
import { KillDialog } from "@/components/KillDialog"
import { StatusDot } from "@/components/StatusDot"
import { Terminal } from "@/components/Terminal"
import { Button } from "@/components/ui/button"
import { agentLabel } from "@/lib/status"
import { getMode, setMode as rememberMode, type DisplayMode } from "@/lib/transcript"
import { AgentChat } from "@/components/AgentChat"
import type { Agent, Project } from "@/types"

// AgentView is the right side when an agent is selected: header, pending approval, terminal, reply box.
export function AgentView({ agent, project }: { agent: Agent; project?: Project }) {
  const terminalControl = useRef<TerminalControl | null>(null)
  const [ownership, setOwnership] = useState<TerminalOwnership>({ owner: false, cols: 0, rows: 0 })
  const [worktrees, setWorktrees] = useState(false)
  const [log, setLog] = useState<string>()
  const action = (data: string) => act(post<{text: string}>('/api/pane/action', { pane: agent.pane, data }).then(r => { if (data === 'log') setLog(r.text) }))
  const [killOpen, setKillOpen] = useState(false)
  const label = agentLabel(agent)
  const [touch] = useState(() => matchMedia("(pointer: coarse)").matches)
  // Chat reads the harness transcript; Terminal is always mounted so its connection and ownership never change.
  const chatSupported = agent.agent === "claude" || agent.agent === "pi"
  const [chatOk, setChatOk] = useState(chatSupported)
  const [mode, setModeState] = useState<DisplayMode>(() => getMode(agent.pane, chatSupported))
  const chat = chatOk && mode === "chat"
  const chooseMode = (m: DisplayMode) => { setModeState(m); rememberMode(agent.pane, m) }
  const [connected, setConnected] = useState(true)
  const keyboardInput = useRef<HTMLTextAreaElement | null>(null)
  const [keyboardOpen, setKeyboardOpen] = useState(false)
  const toggleKeyboard = () => {
    const input = keyboardInput.current
    if (!input) return
    if (document.activeElement === input) input.blur()
    else input.focus({ preventScroll: true })
  }

  return (
    <Panel className="h-full" bodyClass="flex flex-col overflow-hidden" title={<span data-agent-header tabIndex={-1} className="flex min-w-0 items-center gap-1.5" title={`${agent.branch || ""} · ${agent.path}`}><StatusDot status={agent.status} /><span className="max-w-[45%] shrink-0 truncate">{label}</span><span className="min-w-0 truncate text-[11px] font-normal text-muted-foreground">{agent.agent} · {agent.status}</span><span className="hidden min-w-0 truncate text-[11px] font-normal text-muted-foreground sm:inline">{agent.branch ? `${agent.branch} · ` : ""}{agent.path}</span></span>} right={
      <div className="flex items-center gap-1"><span className="max-w-28 text-[10px] leading-tight text-muted-foreground max-sm:hidden">{!ownership.owner && ownership.cols > 0 ? `Viewing at ${ownership.cols}x${ownership.rows} ·` : ''}</span><div role="group" aria-label="Display" className="inline-flex rounded-md border p-0.5"><Button size="xs" variant={chat ? "ghost" : "secondary"} aria-pressed={!chat} onClick={() => chooseMode("terminal")}><TerminalIcon className="size-3" /><span className="max-sm:sr-only">Terminal</span></Button><span title={chatOk ? undefined : "Chat needs a claude or pi agent with a readable transcript"}><Button size="xs" variant={chat ? "secondary" : "ghost"} aria-pressed={chat} disabled={!chatOk} onClick={() => chooseMode("chat")}><MessageSquare className="size-3" /><span className="max-sm:sr-only">Chat</span></Button></span></div><Button size="xs" variant="outline" onClick={() => ownership.owner ? terminalControl.current?.release() : terminalControl.current?.handoff()}><ArrowRightLeft className="size-3" />{ownership.owner ? 'Release' : 'Handoff'}</Button><DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon-xs" aria-label="Agent options"><MoreHorizontal /></Button></DropdownMenuTrigger><DropdownMenuContent><DropdownMenuItem disabled>{agent.branch || agent.path}</DropdownMenuItem><DropdownMenuItem disabled={!chatOk} onSelect={() => chooseMode(chat ? "terminal" : "chat")}>Display: {chat ? "Terminal" : "Chat"}</DropdownMenuItem><DropdownMenuItem onSelect={() => setWorktrees(true)}>Worktrees…</DropdownMenuItem><DropdownMenuItem onSelect={() => void action('pin')}>{agent.pinned ? 'Unpin' : 'Pin'}</DropdownMenuItem><DropdownMenuItem onSelect={() => void action('log')}>Agent log</DropdownMenuItem><DropdownMenuItem onSelect={() => { if (confirm('Restore last checkpoint? Current changes will be backed up.')) void action('undo') }}>Undo last turn…</DropdownMenuItem><DropdownMenuItem onSelect={() => { if (confirm('Clear subagents?')) void action('clear-subagents') }}>Clear subagents</DropdownMenuItem><DropdownMenuItem className="text-destructive" onSelect={() => setKillOpen(true)}>Kill agent…</DropdownMenuItem></DropdownMenuContent></DropdownMenu></div>
    }>
      <div className="flex min-h-0 flex-1 flex-col">
      {agent.dialog && !chat && (
        <div className="m-2 shrink-0 rounded-lg border border-l-4 border-l-amber-500 bg-card p-2">
          <DialogCard key={agent.pane + JSON.stringify(agent.dialog)} pane={agent.pane} dialog={agent.dialog} disabled={!connected} />
        </div>
      )}

      {log !== undefined && <div className="max-h-64 overflow-auto p-3"><Button size="xs" onClick={() => setLog(undefined)}>Close log</Button><pre className="whitespace-pre-wrap text-xs">{log || 'No recent activity.'}</pre></div>}
      <div className={chat ? "hidden" : "flex min-h-0 flex-1 flex-col"}>
        <Terminal key={agent.pane} pane={agent.pane} label={label} controlRef={terminalControl} onOwnership={setOwnership} onConnection={setConnected} keyboardInputRef={keyboardInput} onKeyboardChange={setKeyboardOpen} />
        {touch && <KeyBar pane={agent.pane} disabled={!connected} keyboardOpen={keyboardOpen} onToggleKeyboard={toggleKeyboard} />}
      </div>
      {chat && <AgentChat key={agent.pane} project={project} pane={agent.pane} onOpenTerminal={() => chooseMode("terminal")} onUnavailable={() => setChatOk(false)} />}
      {worktrees && <Worktrees onClose={() => setWorktrees(false)} />}
      {killOpen && <KillDialog pane={agent.pane} name={label} onOpenChange={setKillOpen} />}
      </div>
    </Panel>
  )
}
