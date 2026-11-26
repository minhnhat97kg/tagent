import { Gauge } from "lucide-react"
import type { Overview } from "@/types"

const compact = (n: number) => Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(n)
// A quota bar is neutral: a full bar is normal use, not an alert. Red is reserved for a real block (none is reported here).
const level = (n: number | null) => (n === null ? "bg-muted-foreground/30" : "bg-foreground/60")

export function Meter({ label, value }: { label: string; value: number | null }) {
  const v = value === null ? 0 : Math.max(0, Math.min(100, value))
  return <div className="flex min-w-0 flex-1 items-center gap-1.5" title={`${label}: ${value === null ? "unknown" : `${Math.round(value)}% used`}`}>
    <span className="w-5 shrink-0 text-[10.5px] text-muted-foreground">{label}</span>
    <div className="h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-muted"><div className={`h-full rounded-full ${level(value)}`} style={{ width: `${v}%` }} /></div>
    <span className="w-8 shrink-0 text-right text-[11.5px] font-medium tabular-nums">{value === null ? "–" : `${Math.round(value)}%`}</span>
  </div>
}

// Tokens per day as plain bars: one series, muted, today emphasised; the exact numbers live in the tooltip.
function Bars({ days }: { days: { date: string; tokens: number }[] }) {
  const max = Math.max(1, ...days.map(d => d.tokens))
  return <div className="flex h-8 items-end gap-[2px]" role="img" aria-label={`Tokens per day, last ${days.length} days`}>
    {days.map((d, i) => <div key={d.date} title={`${d.date}: ${compact(d.tokens)}`} className={`min-w-0 flex-1 rounded-t-[2px] ${i === days.length - 1 ? "bg-brand" : "bg-brand/25"}`}
      style={{ height: `${Math.max(d.tokens ? 8 : 2, (d.tokens / max) * 100)}%` }} />)}
  </div>
}

// OverviewPanel is the usage card: provider limits (5h and weekly windows) and token spend.
export function OverviewPanel({ overview, className, days = 7, onDays }: { overview?: Overview; className?: string; days?: number; onDays?: (days: number) => void }) {
  const total = overview?.tokens.days.reduce((s, d) => s + d.tokens, 0) ?? 0
  return <section className={`rounded-lg border bg-card ${className ?? ""}`} aria-label="Usage">
    <header className="flex h-8 items-center gap-1.5 border-b px-2.5">
      <span className="flex size-5 items-center justify-center rounded-md bg-muted"><Gauge className="size-3.5 text-brand" /></span>
      <h2 className="mr-auto text-[12.5px] font-semibold">Usage</h2>
      <a href="#/routing" className="text-[11px] text-muted-foreground underline">Routing →</a>
      <div className="flex rounded-md bg-muted p-0.5 text-[10.5px]" role="group" aria-label="Range">
        {[1, 7, 30].map(d => <button key={d} aria-pressed={days === d} onClick={() => onDays?.(d)}
          className={`rounded px-1.5 py-px tabular-nums ${days === d ? "bg-card font-semibold shadow-sm" : "text-muted-foreground"}`}>{d}d</button>)}
      </div>
    </header>
    {!overview ? <p className="p-2.5 text-xs text-muted-foreground">Loading usage…</p> : <div className="flex flex-col gap-2.5 p-2.5">
      {overview.providers.map(p => <div key={p.name} className="flex flex-col gap-1">
        <div className="flex items-center gap-1.5 text-[12px] font-medium">{p.name}{p.stale && <span className="text-[10.5px] font-normal text-muted-foreground">· stale</span>}</div>
        <div className="flex gap-3"><Meter label="5h" value={p.usedPercent} /><Meter label="7d" value={p.weeklyPercent} /></div>
      </div>)}
      <div className="flex items-end gap-3 border-t pt-2.5">
        <div className="shrink-0">
          <div className="text-[10.5px] text-muted-foreground">Tokens today</div>
          <div className="text-[15px] font-semibold leading-tight tabular-nums">{compact(overview.tokens.today)}</div>
          <div className="text-[10.5px] text-muted-foreground tabular-nums">{compact(total)} in {days}d</div>
        </div>
        <div className="min-w-0 flex-1"><Bars days={overview.tokens.days} /></div>
      </div>
    </div>}
  </section>
}
