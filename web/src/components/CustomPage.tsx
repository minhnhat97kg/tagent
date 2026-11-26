import { enc } from "@/hooks/useRoute"
import { CustomPane } from "@/components/CustomPane"
import type { ExtInfo } from "@/lib/ext"

// CustomPage shows the custom panes (<config dir>/ui/*.html) as tabs; one pane at a time, keyed by name so a switch is a new frame.
export function CustomPage({ exts, name, reload }: { exts: ExtInfo[]; name?: string; reload: () => void }) {
  const names = exts.map(e => e.name)
  if (names.length === 0 || (name !== undefined && !names.includes(name))) {
    return <div className="p-6 text-sm text-muted-foreground">No custom pane named here. Panes are HTML files in the <code>ui</code> folder next to <code>config.conf</code>; reload after adding one.</div>
  }
  const cur = name ?? names[0]
  return (
    <div data-max="custom" className="flex min-h-0 min-w-0 flex-1 flex-col gap-2">
      <nav className="flex gap-1 overflow-x-auto" aria-label="Custom panes">
        {names.map(n => <a key={n} href={`#/custom/${enc(n)}`} aria-current={n === cur ? "page" : undefined} className={`shrink-0 rounded-md px-2 py-1 text-[13px] ${n === cur ? "bg-accent font-medium" : "text-muted-foreground hover:text-foreground"}`}>{n}</a>)}
      </nav>
      <CustomPane key={cur} info={exts.find(e => e.name === cur)!} reload={reload} />
    </div>
  )
}
