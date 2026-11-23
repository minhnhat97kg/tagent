import type { FormEvent, ReactNode } from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"
import { Bot, ChevronRight, CircleAlert } from "lucide-react"
import { cn } from "@/lib/utils"

// Shared look of the agent chat and the receptionist chat.

export const chatColumn = "mx-auto w-full max-w-3xl"

// Bubble: the user's message is a soft bubble on the right; replies are plain text beside a small avatar.
export function Bubble({ who, label, children }: { who: "user" | "assistant" | "error"; label?: string; children: ReactNode }) {
  if (who === "user") return <div className="ml-auto w-fit max-w-[85%] rounded-2xl rounded-br-md bg-muted px-3 py-1.5 text-sm break-words">{children}</div>
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
    <summary className="flex cursor-pointer list-none items-center gap-2 px-2.5 py-1 text-muted-foreground hover:text-foreground [&::-webkit-details-marker]:hidden">
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
  return <form className={cn(chatColumn, "relative px-3 pb-1.5 pt-0.5")} onSubmit={(e: FormEvent) => { e.preventDefault(); onSubmit() }}>
    {above}
    <div title={hint} className="flex items-center gap-1 rounded-xl border bg-background py-0.5 pl-1.5 pr-1 transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/20">{children}</div>
  </form>
}

export const composerInput = "max-h-40 min-h-8 flex-1 resize-none self-center bg-transparent px-1 py-1.5 text-sm leading-5 outline-none [field-sizing:content] placeholder:text-muted-foreground disabled:opacity-60"
export const sendButton = "size-7 shrink-0 self-end rounded-full max-md:size-9"

// Markdown: GitHub-flavoured (tables, task lists, strikethrough); raw HTML is dropped. Links open in a new tab.
export function Markup({ text }: { text: string }) {
  return <div className="space-y-2 [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:font-mono [&_code]:text-[12px] [&_h1]:text-base [&_h1]:font-semibold [&_h2]:text-[15px] [&_h2]:font-semibold [&_h3]:font-semibold [&_hr]:my-2 [&_ol]:list-decimal [&_ol]:pl-5 [&_pre]:overflow-auto [&_pre]:rounded-lg [&_pre]:bg-muted [&_pre]:p-2.5 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_table]:block [&_table]:overflow-x-auto [&_td]:border [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:bg-muted/50 [&_th]:px-2 [&_th]:py-1 [&_th]:text-left [&_ul]:list-disc [&_ul]:pl-5 [&_li]:my-0.5">
    <Markdown skipHtml remarkPlugins={[remarkGfm]} components={{ a: ({ node: _n, ...p }) => <a {...p} target="_blank" rel="noreferrer" /> }}>{text}</Markdown>
  </div>
}
