import { STATUS_ICON } from "@/lib/status"
import { cn } from "@/lib/utils"

export function StatusDot({ status, className }: { status: string; className?: string }) {
  const { icon: Icon, cls } = STATUS_ICON[status] ?? STATUS_ICON.idle!
  return (
    <span title={status} className={cn("flex w-3.5 shrink-0 justify-center", className)}>
      <Icon aria-hidden className={cn("size-3.5", cls)} />
    </span>
  )
}
