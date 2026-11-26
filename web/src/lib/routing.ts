import type { RoutingCandidate, RoutingView } from "@/types"

type Provider = RoutingView["providers"][number]
export const harnessName: Record<string, string> = { claude: "Claude", codex: "Codex", pi: "pi" }

// A ref is harness:model, with an optional own threshold as @N (percent) and an optional #effort level on the end.
const stripEffort = (ref: string) => ref.replace(/#[a-z0-9]+$/, "")
export function parseRef(ref: string): { harness: string; model: string; until: number | null; effort: string } {
  const effort = /#([a-z0-9]+)$/.exec(ref)?.[1] ?? ""
  const bare = stripEffort(ref)
  const i = bare.indexOf(":")
  const rest = i < 0 ? bare : bare.slice(i + 1)
  const m = /@(\d+)$/.exec(rest)
  return { harness: i < 0 ? "" : bare.slice(0, i), model: m ? rest.slice(0, m.index) : rest, until: m ? Number(m[1]) : null, effort }
}
// withUntil sets (or with "" removes) the own threshold of a ref, keeping its effort.
export function withUntil(ref: string, text: string): string {
  const base = stripEffort(ref).replace(/@\d+$/, "")
  const bare = text === "" ? base : `${base}@${text}`
  const effort = parseRef(ref).effort
  return effort ? `${bare}#${effort}` : bare
}
// withEffort sets (or with "" removes) the effort level of a ref, keeping its threshold.
export const withEffort = (ref: string, level: string) => level === "" ? stripEffort(ref) : `${stripEffort(ref)}#${level}`
export function reorder<T>(xs: T[], from: number, to: number): T[] {
  const r = [...xs]
  const [x] = r.splice(from, 1)
  r.splice(to, 0, x)
  return r
}
export const candidateName = (ref: string) => { const p = parseRef(ref); return `${harnessName[p.harness] ?? p.harness} ${p.model}` }
// flow is the one-line chain: each candidate with its threshold, then what happens when all are exhausted.
export function flow(candidates: RoutingCandidate[], whenExhausted: string): string {
  const steps = candidates.map(c => c.threshold === null ? candidateName(c.ref) : `${candidateName(c.ref)} until ${c.threshold}%`)
  return [...steps, whenExhausted === "first" ? "run first" : "block"].join(" -> ")
}
// spillReset is when a spilled candidate clears: the latest reset among the windows over its threshold.
export function spillReset(p: Provider | undefined, threshold: number | null): string | null {
  if (!p || threshold === null) return null
  const resets = [p.five, p.week].filter(w => w.usedPercent !== null && w.usedPercent >= threshold && w.resetsAt).map(w => w.resetsAt as string).sort()
  return resets[resets.length - 1] ?? null
}

