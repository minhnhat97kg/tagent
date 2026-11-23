import { useEffect, useRef, useState, type KeyboardEvent } from "react"
import { toast } from "sonner"
import { post } from "@/api"
import { Button } from "@/components/ui/button"
import { ArrowUp } from "lucide-react"

const KEYS: [string, string][] = [
  ["Esc", "\x1b"],
  ["Ctrl-C", "\x03"],
  ["↑", "\x1b[A"],
  ["↓", "\x1b[B"],
  ["Tab", "\t"],
  ["Enter", "\r"],
]

// Composer types a message into an agent pane, plus keys the terminal needs.
export function Composer({ pane, name }: { pane: string; name: string }) {
  const [text, setText] = useState("")
  const [busy, setBusy] = useState(false)
  const ref = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    const t = ref.current
    if (!t) return
    t.style.height = "auto"
    t.style.height = `${Math.min(t.scrollHeight, 200)}px`
  }, [text])

  async function send() {
    const v = text.trim()
    if (!v || busy) return
    setBusy(true)
    try {
      await post("/api/pane/send", { pane, data: v })
      setText("")
    } catch (e) {
      toast.error((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function key(data: string) {
    try {
      await post("/api/pane/input", { pane, data })
    } catch (e) {
      toast.error((e as Error).message)
    }
  }

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      void send()
    }
  }

  return (
    <div className="rounded-2xl border bg-card p-3 shadow-sm focus-within:ring-2 focus-within:ring-ring/40">
      <textarea
        ref={ref}
        rows={1}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder={`Reply to ${name}…`}
        className="block w-full resize-none bg-transparent text-sm outline-none placeholder:text-muted-foreground"
      />
      <div className="mt-2 flex items-center justify-between gap-2">
        <div className="flex flex-wrap gap-1">
          {KEYS.map(([label, data]) => (
            <Button key={label} variant="ghost" size="xs" className="text-xs" onClick={() => void key(data)}>
              {label}
            </Button>
          ))}
        </div>
        <Button size="icon" className="rounded-full" disabled={!text.trim() || busy} onClick={() => void send()} aria-label="Send">
          <ArrowUp />
        </Button>
      </div>
    </div>
  )
}
