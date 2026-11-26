import type { Role } from "@/types"

// Models already in use across roles, for the model suggestions.
export function modelsIn(roles: Role[]): string[] {
  return [...new Set(roles.map(r => r.model).filter(Boolean))].sort()
}

export function usedBySummary(role: Role): string {
  const used = role.usedBy ?? []
  return used.length ? used.join(", ") : "not used by any status or workflow"
}

export const roleTemplate = (name: string) => `# ${name}\n\nDescribe how this agent works.\n`

// Legacy harness files and global files are read-only here; copy them to the repo to edit.
export const readOnlySource = (source: string) => source === "legacy"

// What the user edits: harness, model, effort and instructions. Live fields (running, usedBy) are left out.
export const editableKey = (r: Role) => JSON.stringify([r.harness, r.tier ?? "", r.model, r.effort, r.instructions, r.skills, r.mcp, r.tools, r.allow, r.deny])

// Context text to list. Skills and MCP: empty is the harness default, "none" is no items.
// Tools, allow and deny: empty is no items.
export function contextList(text: string, nullable: boolean): string[] | undefined {
  const t = text.trim()
  if (!t) return nullable ? undefined : []
  if (nullable && t === "none") return []
  return t.split(",").map(s => s.trim()).filter(Boolean)
}
// Save always sends all five context keys: a cleared field is saved as default (null) or none ([]).
export const withContext = (r: Role) => ({ ...r, skills: r.skills ?? null, mcp: r.mcp ?? null, tools: r.tools ?? [], allow: r.allow ?? [], deny: r.deny ?? [] })
export const contextText = (v: string[] | undefined, nullable: boolean) => v === undefined ? "" : v.length || !nullable ? v.join(", ") : "none"
