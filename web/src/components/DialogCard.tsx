import { useRef, useState } from "react"
import { paneInput } from "@/lib/paneInput"
import { Button } from "@/components/ui/button"
import type { Dialog } from "@/types"

export function DialogCard({ pane, dialog, disabled = false }: { pane: string; dialog: Dialog; disabled?: boolean }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const lock = useRef(false)
  async function answer(data: string) {
    if (lock.current || disabled) return
    lock.current = true; setBusy(true); setError("")
    try { await paneInput(pane, "answer", data, { dialog }) }
    catch (e) { setError((e as Error).message) }
    finally { lock.current = false; setBusy(false) }
  }
  return <div className="flex flex-col gap-2">
    <p className="text-sm font-medium whitespace-pre-wrap">{dialog.question}</p>
    <p role="status" className="text-xs">{error || (busy ? "Sending…" : "Waiting for your answer")}</p>
    <div className="flex flex-wrap gap-1">{(dialog.options ?? []).map(o =>
      <Button key={o.Key} size="sm" className="max-md:min-h-11" variant="outline" disabled={busy || disabled} onClick={() => void answer(o.Key)}>{o.Label}</Button>,
    )}</div>
  </div>
}
