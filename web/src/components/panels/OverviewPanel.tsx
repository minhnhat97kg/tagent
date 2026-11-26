import { Fragment, useState } from "react"
import { Gauge, ListChecks } from "lucide-react"
import { Button } from "@/components/ui/button"
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuLabel, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { PaneHeader } from "@/components/PaneChrome"
import { useMedia } from "@/hooks/useMedia"
import { useSize } from "@/hooks/useSize"
import { loadHidden, saveHidden } from "@/lib/usageSelection"
import type { Overview } from "@/types"

const compact = (n: number) => Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(n)
// A quota bar is neutral: a full bar is normal use, not an alert. Red is reserved for a real block (none is reported here).
const level = (n: number | null) => (n === null ? "bg-muted-foreground/30" : "bg-foreground/60")

// marker draws a tick at a percent (a spill threshold) over the bar.
export function Meter({ label, value, labelClass = "w-5", marker }: { label: string; value: number | null; labelClass?: string; marker?: number | null }) {
  const v = value === null ? 0 : Math.max(0, Math.min(100, value))
  return <div className="flex min-w-0 flex-1 items-center gap-1.5" title={`${label}: ${value === null ? "unknown" : `${Math.round(value)}% used`}`}>
    {label && <span className={`${labelClass} shrink-0 text-[10.5px] text-muted-foreground`}>{label}</span>}
    <div className="relative h-1.5 min-w-0 flex-1 overflow-hidden rounded-full bg-muted"><div className={`h-full rounded-full ${level(value)}`} style={{ width: `${v}%` }} />
      {marker != null && <span className="absolute inset-y-0 w-px bg-foreground" style={{ left: `${marker}%` }} />}</div>
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

type HarnessRow = NonNullable<Overview["harnesses"]>[number]

// usageGroups puts the configured rows under their harness, each with the id its selection is saved by and a name.
function usageGroups(rows: HarnessRow[]) {
  return [...new Set(rows.map(r => r.harness))].map(harness => {
    const own = rows.filter(r => r.harness === harness)
    return { harness, rows: own.map((r, i) => ({ ...r, id: JSON.stringify([harness, r.label || i]), name: r.label || (own.length > 1 ? `Usage ${i + 1}` : "Usage") })) }
  })
}

// OverviewPanel is the usage card: provider limits (5h and weekly windows), the configured harness usage and token spend.
// Under 200px of height (desktop only) the token spend goes; the quota meters stay.
export function OverviewPanel({ overview, className, days = 7, onDays }: { overview?: Overview; className?: string; days?: number; onDays?: (days: number) => void }) {
  const total = overview?.tokens.days.reduce((s, d) => s + d.tokens, 0) ?? 0
  const [ref, { h }] = useSize<HTMLElement>()
  const desktop = useMedia("(min-width: 768px)")
  const short = desktop && h > 0 && h < 200
  const [hidden, setHidden] = useState(loadHidden)
  const shows = (id: string) => !hidden.includes(id)
  const toggle = (id: string) => {
    const next = shows(id) ? [...hidden, id] : hidden.filter(x => x !== id)
    setHidden(next)
    saveHidden(next)
  }
  const providers = overview?.providers.map(p => ({ ...p, id: `provider:${p.name}` })) ?? []
  const groups = usageGroups(overview?.harnesses ?? [])
  const item = (id: string, name: string) => <DropdownMenuCheckboxItem key={id} className="pointer-coarse:py-2.5" checked={shows(id)}
    onCheckedChange={() => toggle(id)} onSelect={e => e.preventDefault()}>{name}</DropdownMenuCheckboxItem>
  return <section ref={ref} className={`rounded-lg border bg-card ${className ?? ""}`} aria-label="Usage">
    <PaneHeader className="min-h-8 border-b px-2.5">
      {/* One row at any width: the title truncates; the range control sits in the body so it never wraps the header. */}
      <div className="flex min-w-0 flex-1 items-center gap-1.5">
      <span className="flex size-5 shrink-0 items-center justify-center rounded-md bg-muted"><Gauge className="size-3.5 text-brand" /></span>
      <h2 className="mr-auto min-w-0 truncate text-[12.5px] font-semibold">Usage</h2>
      <a href="#/routing" className="shrink-0 whitespace-nowrap text-[11px] text-muted-foreground underline">Routing →</a>
      <DropdownMenu>
        <DropdownMenuTrigger asChild><Button variant="ghost" size="icon-xs" className="shrink-0 pointer-coarse:size-8" aria-label="Choose usage to show" title="Choose usage to show"><ListChecks /></Button></DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {providers.map(p => <Fragment key={p.id}><DropdownMenuLabel>{p.name}</DropdownMenuLabel>{item(p.id, "5h and 7d limits")}</Fragment>)}
          {groups.map(g => <Fragment key={g.harness}><DropdownMenuLabel>{g.harness} · harness</DropdownMenuLabel>{g.rows.map(r => item(r.id, r.name))}</Fragment>)}
        </DropdownMenuContent>
      </DropdownMenu>
      </div>
    </PaneHeader>
    <div className="flex justify-end px-2.5 pt-2.5">
      <div className="flex rounded-md bg-muted p-0.5 text-[10.5px]" role="group" aria-label="Range">
        {[1, 7, 30].map(d => <button key={d} aria-pressed={days === d} onClick={() => onDays?.(d)}
          className={`rounded px-1.5 py-px tabular-nums ${days === d ? "bg-card font-semibold shadow-sm" : "text-muted-foreground"}`}>{d}d</button>)}
      </div>
    </div>
    {!overview ? <p className="p-2.5 text-xs text-muted-foreground">Loading usage…</p> : <div className="flex flex-col gap-2.5 p-2.5">
      {providers.filter(p => shows(p.id)).map(p => <div key={p.name} className="flex flex-col gap-1">
        <div className="flex items-center gap-1.5 text-[12px] font-medium">{p.name}{p.stale && <span className="text-[10.5px] font-normal text-muted-foreground">· stale</span>}</div>
        <div className="flex gap-3"><Meter label="5h" value={p.usedPercent} /><Meter label="7d" value={p.weeklyPercent} /></div>
      </div>)}
      {groups.map(g => {
        const rows = g.rows.filter(r => shows(r.id))
        return rows.length > 0 && <div key={g.harness} className="flex flex-col gap-1" role="group" aria-label={`${g.harness} usage`}>
          <div className="truncate text-[12px] font-medium">{g.harness}</div>
          {rows.map((r, i) => r.error
            ? <div key={i} className="flex gap-1.5 text-[11px] text-muted-foreground" title="The usage command failed or did not print a percentage">{r.label && <span className="max-w-24 truncate">{r.label}</span>}<span className="ml-auto">error</span></div>
            : <div key={i} className="flex"><Meter label={r.label} labelClass="max-w-24 truncate" value={r.usedPercent} /></div>)}
        </div>
      })}
      {!short && <div className="flex items-end gap-3 border-t pt-2.5">
        <div className="shrink-0">
          <div className="text-[10.5px] text-muted-foreground">Tokens today</div>
          <div className="text-[15px] font-semibold leading-tight tabular-nums">{compact(overview.tokens.today)}</div>
          <div className="text-[10.5px] text-muted-foreground tabular-nums">{compact(total)} in {days}d</div>
        </div>
        <div className="min-w-0 flex-1"><Bars days={overview.tokens.days} /></div>
      </div>}
    </div>}
  </section>
}
