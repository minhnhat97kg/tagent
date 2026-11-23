import { Loader2, MessageSquarePlus, TriangleAlert } from "lucide-react"
import { AgentChat } from "@/components/AgentChat"
import { Button } from "@/components/ui/button"
import type { TicketChat as Info } from "@/hooks/useTicketChat"
import { enc, nav } from "@/hooks/useRoute"
import type { Project } from "@/types"

// TicketChat is the Chat tab of a ticket: the ticket's own agent pane. It is the
// same conversation as an agent's Chat view — the harness transcript, fed through
// the pane queue — with one extra action: a human can save a message to the
// ticket as a comment, so what was decided in chat survives the pane.
export function TicketChat({ info, error, starting, onStart, onReload, onSave, project }: {
  info?: Info
  error: string
  starting: boolean
  onStart: () => void
  onReload: () => void
  onSave: (text: string) => void
  project?: Project
}) {
  if (error && !info) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!info) return <p className="flex items-center gap-2 p-3 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" />Looking for this ticket's chat…</p>
  // Normally unreachable: the tab is disabled while chat is unavailable. Kept so a
  // pane whose harness changes under the tab still explains itself.
  if (!info.available) return <div role="status" className="m-3 rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 text-sm">
    <p className="flex items-center gap-1.5 font-medium text-amber-700 dark:text-amber-400"><TriangleAlert className="size-4" />Chat is unavailable</p>
    <p className="mt-1 text-muted-foreground">{info.reason || "This ticket's chat cannot be read."}</p>
  </div>
  if (!info.pane) return <div className="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
    <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground"><MessageSquarePlus className="size-5" /></span>
    <div className="space-y-1">
      <p className="text-sm font-medium">No chat for this ticket yet</p>
      <p className="max-w-sm text-xs text-muted-foreground">A claude or pi agent starts with the ticket's description, status, comments and workflow step, and answers here. Messages wait in its queue while it is busy.</p>
    </div>
    {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
    <Button size="sm" disabled={starting} onClick={onStart}>{starting ? <Loader2 className="animate-spin" /> : <MessageSquarePlus />}{starting ? "Starting…" : "Start chat"}</Button>
  </div>
  return <>
    {error && <p role="alert" className="px-3 pt-2 text-xs text-destructive">{error}</p>}
    <AgentChat pane={info.pane} project={project} onOpenTerminal={() => nav("/agent/" + enc(info.pane))} onUnavailable={onReload} onSave={onSave} />
  </>
}
