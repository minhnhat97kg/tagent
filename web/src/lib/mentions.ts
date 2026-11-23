import type { Agent, Task, Workflow } from "@/types"

// What `@` can tag in the agent chat: other agents, tickets and workflows. `#` tags tickets only.
export interface Mention { kind: "agent" | "task" | "workflow"; key: string; label: string; detail: string; insert: string }

const shortId = (id: string) => (/^\d+$/.test(id) ? id : id.slice(-4))
const title = (t: Task) => t.title || t.text.split("\n")[0] || "Untitled"
const clip = (s: string, n: number) => (s.length > n ? s.slice(0, n - 1) + "…" : s)

export function mentionItems(agents: Agent[], tasks: Task[], workflows: Workflow[], self?: string): Mention[] {
  return [
    ...agents.filter(a => a.pane !== self).map<Mention>(a => ({ kind: "agent", key: `${a.name} ${a.pane}`, label: a.name || a.pane, detail: `${a.agent} · ${a.status}`, insert: `@${a.name || a.pane}` })),
    ...tasks.filter(t => t.status !== "done").map<Mention>(t => ({ kind: "task", key: `${t.id} ${shortId(t.id)} ${title(t)}`, label: `#${shortId(t.id)} ${clip(title(t), 48)}`, detail: t.status, insert: `task #${shortId(t.id)} (${clip(title(t), 60)})` })),
    ...workflows.filter(w => !w.error).map<Mention>(w => ({ kind: "workflow", key: w.name, label: w.name, detail: w.description || `${w.jobs.length} steps`, insert: `workflow "${w.name}"` })),
  ]
}

// The `@word` or `#word` being typed right before the caret, if any.
export function mentionQuery(text: string, caret: number): { trigger: "@" | "#"; start: number; query: string } | null {
  const m = /(^|\s)([@#])([^\s@#]*)$/.exec(text.slice(0, caret))
  return m ? { trigger: m[2] as "@" | "#", start: caret - m[3]!.length - 1, query: m[3]!.toLowerCase() } : null
}

export function filterMentions(items: Mention[], q: { trigger: "@" | "#"; query: string }, limit = 8): Mention[] {
  const pool = q.trigger === "#" ? items.filter(i => i.kind === "task") : items
  return pool.filter(i => i.key.toLowerCase().includes(q.query)).slice(0, limit)
}

// Replace the typed `@word` with the chosen mention and a space; returns the new text and caret.
export function applyMention(text: string, caret: number, start: number, m: Mention) {
  const next = text.slice(0, start) + m.insert + " " + text.slice(caret)
  return { text: next, caret: start + m.insert.length + 1 }
}
