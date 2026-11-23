import type { Role } from "@/types"

// Effort levels each harness accepts; mirrors tasks.ValidateEffort in Go.
export function effortLevels(harness: string): string[] {
  switch (harness) {
    case "pi": return ["off", "minimal", "low", "medium", "high", "xhigh", "max"]
    case "claude": return ["low", "medium", "high", "xhigh", "max"]
    case "codex": return ["none", "minimal", "low", "medium", "high", "xhigh", "max"]
    default: return []
  }
}

// Models already in use across roles, for the model suggestions.
export function modelsIn(roles: Role[]): string[] {
  return [...new Set(roles.map(r => r.model).filter(Boolean))].sort()
}

export function usedBySummary(role: Role): string {
  const used = role.usedBy ?? []
  return used.length ? used.join(", ") : "not used by any status or workflow"
}

export const roleTemplate = (name: string) => `---\nname: ${name}\ndescription: \n---\n\nDescribe how this agent works.\n`

// What the user edits: harness, model, effort and instructions. Live fields (running, usedBy) are left out.
export const editableKey = (r: Role) => JSON.stringify([r.harness, r.model, r.effort, r.instructions])
