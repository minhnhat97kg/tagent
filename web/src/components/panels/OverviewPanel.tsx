import { LayoutDashboard } from "lucide-react"
import { ProjectSelect } from "@/components/ProjectSelect"
import { Panel } from "@/components/Panel"
import { nav } from "@/hooks/useRoute"
import { baseName } from "@/lib/status"
import type { Overview } from "@/types"
const compact = (n: number) => Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 }).format(n)
export function OverviewPanel({overview, projects, dir, onDir, className}: {overview?: Overview; projects: string[]; dir: string; onDir: (dir: string) => void; className?: string}) {
 const pct = (n: number | null) => n === null ? "n/a" : `${Math.round(n)}%`
 return <Panel className={className} icon={<LayoutDashboard className="size-4" />} title={`Overview · ${baseName(dir)}`} right={<ProjectSelect value={dir} projects={projects} onChange={d => {onDir(d); nav("/")}} />}>
 {!overview ? <p className="p-2.5 text-xs text-muted-foreground">Loading overview…</p> : <div className="grid grid-cols-3 gap-2 p-2.5 text-xs">
 {overview.providers.map(p => <div key={p.name} className="space-y-1"><div className="text-muted-foreground">{p.name}{p.stale ? " (stale)" : ""}</div><div className="font-semibold">{pct(p.usedPercent)} · {pct(p.weeklyPercent)}</div><div className="h-1 rounded bg-muted"><div className="h-1 rounded bg-zinc-600" style={{width: `${Math.max(0, Math.min(100, p.weeklyPercent ?? 0))}%`}} /></div></div>)}
 <div className="space-y-1"><div className="text-muted-foreground">Tokens today</div><div className="font-semibold">{compact(overview.tokens.today)}</div><div title={overview.tokens.days.map(d => `${d.date}: ${d.tokens}`).join("\n")} className="font-mono text-zinc-600">{overview.tokens.sparkline}</div><div className="text-muted-foreground">7d · {compact(overview.tokens.total7d)}</div></div>
 </div>}
 </Panel>
}
