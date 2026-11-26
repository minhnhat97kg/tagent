import { CircleDashed, Hand, LoaderCircle, MessageSquareDot, Moon, TriangleAlert, type LucideIcon } from "lucide-react"

// One icon per state: working, background work, needs you, failed, finished (unread), asleep.
export const STATUS_ICON: Record<string, { icon: LucideIcon; cls: string }> = {
  running: { icon: LoaderCircle, cls: "text-emerald-500 animate-spin" },
  background: { icon: CircleDashed, cls: "text-emerald-500/70" },
  waiting: { icon: Hand, cls: "text-amber-500" },
  error: { icon: TriangleAlert, cls: "text-red-500" },
  done: { icon: MessageSquareDot, cls: "text-green-600" },
  idle: { icon: Moon, cls: "text-zinc-400" },
}

export const isActive = (status: string) => status === "running" || status === "background"

// shortPrompt is the first line of a prompt, cut for a one-line row. Pasted-content wrappers and system reminders are not the message.
export const shortPrompt = (s: string) => (s.replace(/<system-reminder>[\s\S]*?<\/system-reminder>/g, "").replace(/<\/?pasted_content[^>]*>/g, "").split("\n").find(l => l.trim()) ?? "").trim().slice(0, 120)

export const baseName = (p: string) => p.split("/").filter(Boolean).pop() ?? p

// ago formats a unix time in seconds as "42s", "3m" or "2h" ago.
export function ago(epochSec: number) {
  const s = Math.max(0, Date.now() / 1000 - epochSec)
  return s < 60 ? `${Math.floor(s)}s` : s < 3600 ? `${Math.floor(s / 60)}m` : `${Math.floor(s / 3600)}h`
}

export const agentLabel = (a: { name: string; agent: string }) => a.name || a.agent

// taskOf is the ticket a board agent works on: its pane is named "<role>-<ticket>" (or "<role>#<ticket>"); "" for any other agent.
export const taskOf = (a: { name: string }) => /[-#](\d+)$/.exec(a.name)?.[1] ?? ""
// byTaskOrder puts agents without a task first, then orders by ticket id (stable otherwise).
export const byTaskOrder = (a: { name: string }, b: { name: string }) => {
  const x = taskOf(a), y = taskOf(b)
  return x === y ? 0 : !x ? -1 : !y ? 1 : x.length - y.length || (x < y ? -1 : 1)
}
