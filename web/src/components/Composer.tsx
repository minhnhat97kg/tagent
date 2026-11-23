import { toast } from "sonner"
import { paneInput } from "@/lib/paneInput"

const KEYS = [["Esc", "\x1b"], ["Ctrl-C", "\x03"], ["↑", "\x1b[A"], ["↓", "\x1b[B"], ["←", "\x1b[D"], ["→", "\x1b[C"], ["Tab", "\t"], ["⇧Tab", "\x1b[Z"], ["Enter", "\r"]]

// KeyBar gives touch screens the keys their on-screen keyboard lacks. Typing goes into the terminal itself.
export function KeyBar({ pane, disabled = false }: { pane: string; disabled?: boolean }) {
  return <div className="flex shrink-0 gap-1 overflow-x-auto border-t bg-card px-1.5 py-1 pb-[max(4px,env(safe-area-inset-bottom))]" role="toolbar" aria-label="Terminal keys">
    {KEYS.map(([label, data]) => <button key={label} type="button" disabled={disabled}
      aria-label={`Send ${label} to terminal`} className="h-8 min-w-10 shrink-0 rounded-md border bg-background px-2 font-mono text-[12px] text-zinc-700 disabled:opacity-40"
      onPointerDown={e => e.preventDefault() /* keep the terminal's keyboard open */}
      onClick={() => void paneInput(pane, "input", data).catch(e => toast.error(e.message))}>{label}</button>)}
  </div>
}
