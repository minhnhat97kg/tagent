import type { Overview } from "@/types"
const compact = (n: number) => Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(n)
const pct = (n: number | null) => (n === null ? "–" : `${Math.round(n)}%`)

// OverviewPanel is the usage strip at the top of the rail: provider limits and tokens.
export function OverviewPanel({ overview, className }: { overview?: Overview; className?: string }) {
  return <section className={`rounded-lg border bg-card px-3 py-2.5 ${className ?? ""}`} aria-label="Usage">
    {!overview ? <p className="text-xs text-muted-foreground">Loading usage…</p> : <div className="grid grid-cols-3 gap-3 text-[11.5px]">
      {overview.providers.map(p => <div key={p.name} className="min-w-0 space-y-1" title={`${p.name}: 5h ${pct(p.usedPercent)} · 7d ${pct(p.weeklyPercent)}${p.stale ? " (stale)" : ""}`}>
        <div className="truncate text-muted-foreground">{p.name}</div>
        <div className="font-semibold tabular-nums">{pct(p.usedPercent)}<span className="font-normal text-muted-foreground"> · {pct(p.weeklyPercent)}</span></div>
        <div className="h-1 rounded-full bg-muted"><div className={`h-1 rounded-full ${(p.weeklyPercent ?? 0) > 85 ? "bg-amber-500" : "bg-zinc-700"}`} style={{ width: `${Math.max(0, Math.min(100, p.weeklyPercent ?? 0))}%` }} /></div>
      </div>)}
      <div className="min-w-0 space-y-1" title={[`7d: ${compact(overview.tokens.total7d)}`, ...overview.tokens.days.map(d => `${d.date}: ${d.tokens}`)].join("\n")}>
        <div className="truncate text-muted-foreground">Tokens</div>
        <div className="font-semibold tabular-nums">{compact(overview.tokens.today)}<span className="font-normal text-muted-foreground"> today</span></div>
        <div className="font-mono leading-none text-zinc-500">{overview.tokens.sparkline}</div>
      </div>
    </div>}
  </section>
}
