import { paneInput, reportInputError } from "@/lib/paneInput"

const KEYS = [["Esc", "\x1b"], ["Ctrl-C", "\x03"], ["↑", "\x1b[A"], ["↓", "\x1b[B"], ["←", "\x1b[D"], ["→", "\x1b[C"], ["Tab", "\t"], ["⇧Tab", "\x1b[Z"], ["Enter", "\r"]]

// Touch typing is opt-in; key chips preserve the hidden IME input's focus.
export function KeyBar({ pane, disabled = false, keyboardOpen, onToggleKeyboard }: { pane: string; disabled?: boolean; keyboardOpen: boolean; onToggleKeyboard: () => void }) {
  return <div className="flex shrink-0 gap-1 overflow-x-auto border-t bg-card px-1.5 py-1 pb-[max(4px,env(safe-area-inset-bottom))]" role="toolbar" aria-label="Terminal keys">
    <button type="button" disabled={disabled} aria-label={keyboardOpen ? 'Hide keyboard' : 'Show keyboard'} aria-pressed={keyboardOpen}
      className="h-8 min-w-10 shrink-0 rounded-md border bg-background px-2 text-[16px] text-foreground aria-pressed:bg-accent disabled:opacity-40"
      onPointerDown={e => e.preventDefault()}
      onClick={onToggleKeyboard}>⌨</button>
    {KEYS.map(([label, data]) => <button key={label} type="button" disabled={disabled}
      aria-label={`Send ${label} to terminal`} className="h-8 min-w-10 shrink-0 rounded-md border bg-background px-2 font-mono text-[12px] text-foreground disabled:opacity-40"
      onPointerDown={e => e.preventDefault() /* keep the terminal's keyboard open */}
      onClick={() => void paneInput(pane, "input", data).catch(reportInputError)}>{label}</button>)}
  </div>
}
