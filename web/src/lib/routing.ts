import type { RoutingCandidate, RoutingView } from "@/types"

type Provider = RoutingView["providers"][number]
export const harnessName: Record<string, string> = { claude: "Claude", codex: "Codex", pi: "pi" }

// A ref is harness:model, with an optional own threshold as @N (percent) on the end.
export function parseRef(ref: string): { harness: string; model: string; until: number | null } {
  const i = ref.indexOf(":")
  const rest = i < 0 ? ref : ref.slice(i + 1)
  const m = /@(\d+)$/.exec(rest)
  return { harness: i < 0 ? "" : ref.slice(0, i), model: m ? rest.slice(0, m.index) : rest, until: m ? Number(m[1]) : null }
}
// withUntil sets (or with "" removes) the own threshold of a ref.
export const withUntil = (ref: string, text: string) => { const base = ref.replace(/@\d+$/, ""); return text === "" ? base : `${base}@${text}` }
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

