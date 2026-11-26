import { useContext, type ReactNode } from "react"
import { Maximize2, Minimize2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { PaneChromeContext } from "@/lib/paneChrome"
import { cn } from "@/lib/utils"

// PaneHeader is a pane's header row: the grip, the pane's own controls, and the maximize or minimize link
// at the right end. pane renames the link when the header shows more than one view.
export function PaneHeader({ as: Tag = "header", pane, className, children }: { as?: "header" | "div"; pane?: string; className?: string; children: ReactNode }) {
  const chrome = useContext(PaneChromeContext)
  const id = pane ?? chrome?.id
  const max = chrome?.maximized
  const label = max ? "Minimize" : "Maximize"
  return <Tag data-pane-head onPointerDown={chrome?.onPointerDown}
    className={cn("flex items-center gap-1.5", chrome?.onPointerDown && "cursor-grab touch-none select-none [-webkit-touch-callout:none]", className)}>
    {chrome?.grip}
    {children}
    {chrome && <Button asChild variant="ghost" size="icon-xs"
      // A mouse sees the maximize link on hover or focus; touch and the way back always show.
      className={cn("pointer-coarse:size-8", !max && "pointer-fine:opacity-0 pointer-fine:group-focus-within/pane:opacity-100 pointer-fine:group-hover/pane:opacity-100")}>
      <a href={max ? "#/" : `#/pane/${id}`} aria-label={`${label} ${id} pane`} title={label}>{max ? <Minimize2 /> : <Maximize2 />}</a>
    </Button>}
  </Tag>
}

// NoPaneChrome keeps the headers and dialogs inside a pane from repeating its grip and link.
export function NoPaneChrome({ children }: { children: ReactNode }) {
  return <PaneChromeContext.Provider value={null}>{children}</PaneChromeContext.Provider>
}
