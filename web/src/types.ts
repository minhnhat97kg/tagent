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

export interface State {
  agents: Agent[]
  harnesses: string[] | null
  host: string
  theme?: string
  projects: string[] | null
}

export interface Task {
 title: string
 workflow?: string
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
  statuses: { id: string; role: string; run: string }[]
  workers?: Record<string, { role: string; pane: string; live: boolean }>
  blockedReasons?: Record<string, string>
  workflows?: Workflow[]
  dir: string
  tasks: Task[] | null
  roles: string[] | null
}


export interface Overview { dir: string; project: string; attention: { needed: boolean; text: string; items?: {kind: string; text: string; pane: string | null; count: number}[] }; providers: {name: string; usedPercent: number | null; weeklyPercent: number | null; stale: boolean}[]; tokens: {today: number; total7d: number; sparkline: string; days: {date: string; tokens: number}[]} }
export interface Draft {title: string; goal: string; acceptance: string[]; outOfScope: string[]; workflow: string}
export interface Chat {dir: string; enabled: boolean; revision: number; status: string; recipients?: {Name: string; Pane: string; Role: string}[]; messages: {role: string; text: string; revision: number; tool?: {Calls: {name: string; args: Record<string,string>}[]; Status: string; Output: string}}[]; response: {question: string; answer: string; escalate: string; draft: Draft | null}; workflows: string[]; actions: string[]}
export interface Workflow { name: string; description: string; global: boolean; file: string; error: string; jobs: { id: string; status: string; agent: string; run: string }[] }
