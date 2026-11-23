import { useState } from "react"
import { MoreHorizontal } from "lucide-react"
import { Panel } from "@/components/Panel"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { KeyBar } from "@/components/Composer"
import { DialogCard } from "@/components/DialogCard"
import { KillDialog } from "@/components/KillDialog"
import { StatusDot } from "@/components/StatusDot"
import { Terminal } from "@/components/Terminal"
import { Button } from "@/components/ui/button"
import { agentLabel } from "@/lib/status"
import type { Agent } from "@/types"

// AgentView is the right side when an agent is selected: header, pending approval, terminal, reply box.
export function AgentView({ agent }: { agent: Agent }) {
  const [killOpen, setKillOpen] = useState(false)
  const label = agentLabel(agent)
  const [touch] = useState(() => matchMedia("(pointer: coarse)").matches)
  const [connected, setConnected] = useState(true)

  return (
    <Panel className="h-full" bodyClass="flex flex-col overflow-hidden" title={<span data-agent-header tabIndex={-1} className="flex min-w-0 items-center gap-1.5" title={`${agent.branch || ""} · ${agent.path}`}><StatusDot status={agent.status} /><span className="truncate">{label}</span><span className="text-[11px] font-normal text-muted-foreground">{agent.agent} · {agent.status}</span><span className="hidden min-w-0 truncate text-[11px] font-normal text-muted-foreground sm:inline">{agent.branch ? `${agent.branch} · ` : ""}{agent.path}</span></span>} right={
      <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon-xs" aria-label="Agent options"><MoreHorizontal /></Button></DropdownMenuTrigger><DropdownMenuContent><DropdownMenuItem disabled>{agent.branch || agent.path}</DropdownMenuItem><DropdownMenuItem className="text-destructive" onSelect={() => setKillOpen(true)}>Kill agent…</DropdownMenuItem></DropdownMenuContent></DropdownMenu>
    }>
      <div className="flex min-h-0 flex-1 flex-col">
      {agent.dialog && (
        <div className="m-2 shrink-0 rounded-lg border border-l-4 border-l-amber-500 bg-card p-2">
          <DialogCard key={agent.pane + JSON.stringify(agent.dialog)} pane={agent.pane} dialog={agent.dialog} disabled={!connected} />
        </div>
      )}

      <Terminal key={agent.pane} pane={agent.pane} label={label} onConnection={setConnected} />
      {touch && <KeyBar pane={agent.pane} disabled={!connected} />}
      {killOpen && <KillDialog pane={agent.pane} name={label} onOpenChange={setKillOpen} />}
      </div>
    </Panel>
  )
}
