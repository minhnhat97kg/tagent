import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

// Panel is one card of the shell: a 32px header over a scrolling body.
export function Panel({ icon, title, right, className, bodyClass, children }: {
  icon?: ReactNode
  title: ReactNode
  right?: ReactNode
  className?: string
  bodyClass?: string
  children: ReactNode
}) {
  return (
    <section className={cn("flex min-h-0 flex-col overflow-hidden rounded-lg border bg-card", className)}>
      <header data-pane-head className="flex h-8 shrink-0 items-center gap-1.5 border-b px-2.5">
        {icon && <span className="flex size-5 items-center justify-center rounded-md bg-muted [&_svg]:size-3.5">{icon}</span>}
        <h2 className="min-w-0 truncate text-[12.5px] font-semibold">{title}</h2>
        <div className="ml-auto flex shrink-0 items-center gap-1 text-[11px] text-muted-foreground">{right}</div>
      </header>
      <div className={cn("min-h-0 flex-1 overflow-y-auto", bodyClass)}>{children}</div>
    </section>
  )
}
