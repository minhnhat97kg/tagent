// Shapes from internal/web/server.go. Go structs without json tags (Job, Option,
// Task fields are tagged) keep their Go field names; nil slices arrive as null.
export interface Option { Key: string; Label: string }
export interface Dialog { question: string; options: Option[] | null }

export interface Agent {
  pane: string
  name: string
  agent: string
  status: string
  session: string
  window: string
  path: string
  project: string
  prompt: string
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
  projects: string[] | null
}

export interface Task {
 title: string
 workflow?: string
 comments: { author: string; body: string }[] | null
  id: string
  text: string
  status: string
  assignees: string[] | null
  created: string
  updated: string
  note?: string
}

export interface Project {
  dir: string
  tasks: Task[] | null
  roles: string[] | null
}


export interface Overview { dir: string; project: string; attention: { needed: boolean; text: string }; providers: {name: string; usedPercent: number | null; weeklyPercent: number | null; stale: boolean}[]; tokens: {today: number; total7d: number; sparkline: string; days: {date: string; tokens: number}[]} }
export interface Draft {title: string; goal: string; acceptance: string[]; outOfScope: string[]; workflow: string}
export interface Chat {dir: string; enabled: boolean; revision: number; status: string; messages: {role: string; text: string; revision: number}[]; response: {question: string; answer: string; command: string; escalate: string; draft: Draft | null}; workflows: string[]; actions: string[]}
