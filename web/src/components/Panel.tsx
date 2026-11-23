import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

// Panel is one box of the dashboard: a thin header row over a scrolling body.
export function Panel({
  icon,
  title,
  right,
  className,
  bodyClass,
  children,
}: {
  icon?: ReactNode
  title: ReactNode
  right?: ReactNode
  className?: string
  bodyClass?: string
  children: ReactNode
}) {
  return (
    <section className={cn("flex min-h-0 flex-col overflow-hidden rounded-[10px] border bg-card", className)}>
      <header className="flex h-9 shrink-0 items-center gap-1.5 border-b px-2.5 text-xs">
        {icon && <span className="text-muted-foreground">{icon}</span>}
        <h2 className="min-w-0 truncate text-[13px] font-semibold">{title}</h2>
        <div className="ml-auto flex shrink-0 items-center gap-1 text-muted-foreground">{right}</div>
      </header>
      <div className={cn("min-h-0 flex-1 overflow-y-auto", bodyClass)}>{children}</div>
    </section>
  )
}
