import { useState } from "react"
import { GitBranch } from "lucide-react"
import { Composer } from "@/components/Composer"
import { DialogCard } from "@/components/DialogCard"
import { KillDialog } from "@/components/KillDialog"
import { StatusDot } from "@/components/StatusDot"
import { Terminal } from "@/components/Terminal"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { agentLabel } from "@/lib/status"
import type { Agent } from "@/types"

// AgentView is the right side when an agent is selected: header, pending approval, terminal, reply box.
export function AgentView({ agent }: { agent: Agent }) {
  const [killOpen, setKillOpen] = useState(false)
  const label = agentLabel(agent)

  return (
    <div className="flex h-full min-h-0 flex-col gap-3 p-3">
      <header className="flex flex-wrap items-center gap-2">
        <StatusDot status={agent.status} />
        <h1 className="min-w-0 truncate font-serif text-lg">{label}</h1>
        <Badge variant="secondary">{agent.agent}</Badge>
        <Badge variant="outline">{agent.status}</Badge>
        <div className="flex min-w-0 flex-1 items-center gap-1 text-xs text-muted-foreground">
          <GitBranch className="size-3 shrink-0" />
          <span className="truncate">{agent.branch ? `${agent.branch} · ${agent.path}` : agent.path}</span>
        </div>
        <Button variant="outline" size="sm" className="text-destructive" onClick={() => setKillOpen(true)}>
          Kill
        </Button>
      </header>

      {agent.dialog && (
        <div className="shrink-0 rounded-xl border border-l-4 border-l-amber-500 bg-card p-3">
          <DialogCard pane={agent.pane} dialog={agent.dialog} />
        </div>
      )}

      <Terminal key={agent.pane} pane={agent.pane} />
      <Composer pane={agent.pane} name={label} />
      {killOpen && <KillDialog pane={agent.pane} name={label} onOpenChange={setKillOpen} />}
    </div>
  )
}
