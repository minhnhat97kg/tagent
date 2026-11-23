import type { FormEvent, ReactNode } from "react"
import { Bot, ChevronRight, CircleAlert } from "lucide-react"
import { cn } from "@/lib/utils"

// Shared look of the agent chat and the receptionist chat.

export const chatColumn = "mx-auto w-full max-w-3xl"

// Bubble: the user's message is a soft bubble on the right; replies are plain text beside a small avatar.
export function Bubble({ who, label, children }: { who: "user" | "assistant" | "error"; label?: string; children: ReactNode }) {
  if (who === "user") return <div className="ml-auto w-fit max-w-[85%] rounded-2xl rounded-br-md bg-muted px-3.5 py-2 text-sm break-words">{children}</div>
  const error = who === "error"
  return <div className="flex gap-2.5">
    <span className={cn("mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full border bg-card [&_svg]:size-3.5", error && "border-destructive/40 text-destructive")}>
      {error ? <CircleAlert /> : <Bot />}
    </span>
    <div className={cn("min-w-0 flex-1 text-sm leading-relaxed break-words", error && "text-destructive")}>
      {label && <div className="mb-0.5 text-xs font-medium text-muted-foreground">{label}</div>}
      {children}
    </div>
  </div>
}

// Fold: a one-line card that opens to show details (tool calls, reasoning).
export function Fold({ icon, title, end, open, children }: { icon?: ReactNode; title: ReactNode; end?: ReactNode; open?: boolean; children: ReactNode }) {
  return <details open={open} className="group ml-8 rounded-lg border bg-muted/30 text-xs">
    <summary className="flex cursor-pointer list-none items-center gap-2 px-2.5 py-1.5 text-muted-foreground hover:text-foreground [&::-webkit-details-marker]:hidden">
      <ChevronRight className="size-3 shrink-0 transition-transform group-open:rotate-90" />
      {icon}
      <span className="min-w-0 truncate font-mono">{title}</span>
      {end && <span className="ml-auto flex shrink-0 items-center">{end}</span>}
    </summary>
    <div className="space-y-1.5 border-t p-2.5 [&_pre]:max-h-64 [&_pre]:overflow-auto [&_pre]:whitespace-pre-wrap [&_pre]:font-mono">{children}</div>
  </details>
}

export function Typing({ label }: { label: string }) {
  return <div role="status" className="ml-8 flex items-center gap-2 text-xs text-muted-foreground">
    <span className="flex gap-1" aria-hidden>
      {[0, 150, 300].map(d => <span key={d} className="size-1.5 animate-bounce rounded-full bg-muted-foreground/60" style={{ animationDelay: `${d}ms` }} />)}
    </span>
    {label}
  </div>
}

// Composer: one rounded box holding the textarea and the send button.
export function Composer({ onSubmit, hint, above, children }: { onSubmit: () => void; hint?: string; above?: ReactNode; children: ReactNode }) {
  return <form className={cn(chatColumn, "relative px-3 pb-2 pt-1")} onSubmit={(e: FormEvent) => { e.preventDefault(); onSubmit() }}>
    {above}
    <div className="flex items-end gap-2 rounded-2xl border bg-background px-3 py-1.5 shadow-sm transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/20">{children}</div>
    {hint && <p className="mt-1 px-1 text-[11px] text-muted-foreground max-md:hidden">{hint}</p>}
  </form>
}

export const composerInput = "max-h-40 min-h-9 flex-1 resize-none bg-transparent py-2 text-sm outline-none [field-sizing:content] placeholder:text-muted-foreground disabled:opacity-60"
export const sendButton = "mb-0.5 size-8 shrink-0 rounded-full max-md:size-10"
