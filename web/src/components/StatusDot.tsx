import { STATUS_GLYPH } from "@/lib/status"
import { cn } from "@/lib/utils"

export function StatusDot({ status, className }: { status: string; className?: string }) {
  const g = STATUS_GLYPH[status] ?? STATUS_GLYPH.idle!
  return (
    <span title={status} className={cn("w-3 shrink-0 text-center text-xs leading-none", g.cls, className)}>
      {g.glyph}
    </span>
  )
}
