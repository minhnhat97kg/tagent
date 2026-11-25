// Shapes from internal/web/server.go. Go structs without json tags (Job, Option,
// Task fields are tagged) keep their Go field names; nil slices arrive as null.
export interface Option { Key: string; Label: string }
export interface Dialog { question: string; options: Option[] | null }

export interface Agent {
  pinned?: boolean
  pane: string
  name: string
  agent: string
  status: string
  session: string
  window: string
  path: string
  project: string
  prompt: string
  stuck?: string
  wait?: string
  started?: number
  branch?: string
  hidden?: boolean
  dialog?: Dialog | null
  subagents?: string[] | null
}

// Role is an agent definition from /api/roles: harness, model and effort from board.yaml, instructions from its file.
export interface Role {
  name: string
  harness: string
  model: string
  effort: string
  source: "repo" | "legacy" | "global" | "board"
  path: string
  instructions: string
  running: number
  panes: { pane: string; status: string }[] | null
  usedBy: string[] | null
  // Context: undefined keeps the harness default; [] means none (skills, mcp only).
  skills?: string[]
  mcp?: string[]
  tools?: string[]
  allow?: string[]
  deny?: string[]
}

export interface State {
  agents: Agent[]
  harnesses: string[] | null
  models: Record<string, string[]> | null
  host: string
  projects: string[] | null
}

export interface Task {
 title: string
 workflow?: string
 priority?: "high" | "normal" | "low"
 needs?: string[] | null
 comments: { id?: string; author: string; body: string; created?: string }[] | null
  id: string
  text: string
  status: string
  assignees: string[] | null
  created: string
  updated: string
  note?: string
}

export interface Project {
  defaultWorkflow?: string
  problems?: string[]
  statuses: { id: string; role: string; run: string }[]
  workers?: Record<string, { role: string; pane: string; live: boolean }>
  // Roles whose pane is working on each ticket right now (ticket id -> roles).
  running?: Record<string, string[]>
  blockedReasons?: Record<string, string>
  workflows?: Workflow[]
  dir: string
  tasks: Task[] | null
  roles: string[] | null
}


export interface Overview { dir: string; project: string; attention: { needed: boolean; text: string; items?: {kind: string; text: string; pane: string | null; count: number}[] }; providers: {name: string; usedPercent: number | null; weeklyPercent: number | null; stale: boolean}[]; tokens: {today: number; total7d: number; sparkline: string; days: {date: string; tokens: number}[]} }
export interface Workflow { name: string; description: string; global: boolean; file: string; error: string; jobs: { id: string; status: string; agent: string; run: string }[] }

// RunEntry is one entry into a step of a ticket's workflow run. The dispatch
// details and end are absent on old entries and on the current one.
export interface UsageCount { Input: number; Output: number; CacheRead: number; CacheWrite: number; Total: number }
// Money is an API-equivalent USD estimate; low < high only when the input/output split is unknown.
export interface Money { low: number; high: number }
export interface JobUsage { role: string; harness: string; model: string; effort: string; loops: number; elapsed: number; count: UsageCount; has: boolean; live: boolean; cost?: Money }
export interface TicketUsage { jobs: JobUsage[]; models: { harness: string; model: string; count: UsageCount; has: boolean }[]; total: UsageCount; partial: boolean; elapsed: number; loops: number; cost?: Money; unpriced?: boolean }

export interface RunEntry {
  seq: number
  step: string
  loop: number
  role?: string
  harness?: string
  model?: string
  effort?: string
  pane?: string
  pane_name?: string
  prompt?: string
  start: string
  end?: string
  duration: number
  result: string
  comment?: string
  comment_id?: string
  reason?: string
  outputs?: Record<string, string> | null
  exit_code?: number
  attachments?: { name: string; file: string }[] | null
}

export interface RunView {
  ticket: string
  title: string
  workflow: string
  inputs?: Record<string, string> | null
  status: string
  started: string
  total: number
  entries: RunEntry[] | null
  flow: { job: string; status: string; count: number; current: boolean }[]
}
