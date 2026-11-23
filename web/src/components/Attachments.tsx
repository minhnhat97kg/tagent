import { useEffect, useState } from "react"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { Copy, Download, File, FileDiff, FileImage, FileJson, FileText, X } from "lucide-react"
import { previewText } from "@/lib/attachments"
import { Md } from "./BoardDrawer"

export type Attachment = { ref: string; kind: string; name: string; size: number; author: string; time: string; commentId?: string; error?: string }
const ICONS: Record<string, typeof File> = { image: FileImage, markdown: FileText, diff: FileDiff, json: FileJson }
const type = (a: Attachment) => a.kind.split("/").pop() ?? ""
// comment.md is the full text of a long comment; say so instead of showing the bare filename.
const label = (a: Attachment) => a.name === "comment.md" ? "Full comment" : a.name
const human = (n: number) => n < 1024 ? `${n} B` : n < 1048576 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1048576).toFixed(1)} MB`
// Each attachment is a card; clicking it opens the viewer as an overlay.
export function AttachmentRows({ items, open }: { items: Attachment[]; open: (a: Attachment) => void }) {
  if (!items.length) return null
  return <div className="mt-1.5 grid grid-cols-[repeat(auto-fill,minmax(180px,1fr))] gap-1.5">{items.map(a => {
    const Icon = ICONS[type(a)] ?? File
    return <button type="button" key={a.ref} disabled={!!a.error} title={a.error || `Open ${a.name}`} onClick={() => open(a)}
      className="flex min-w-0 items-center gap-2 rounded-lg border bg-background px-2.5 py-2 text-left shadow-xs hover:border-foreground/30 hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50">
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted"><Icon className="size-4 text-teal-600" /></span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[12.5px] font-medium">{label(a)}</span>
        <span className="block truncate text-[11px] text-muted-foreground">{a.error ? "unavailable" : `${human(a.size)} · ${a.author} · ${new Date(a.time).toLocaleDateString()}`}</span>
      </span>
    </button>
  })}</div>
}
export function AttachmentViewer({ a, url, close }: { a: Attachment; url: string; close: () => void }) {
  const [text, setText] = useState("")
  const [image, setImage] = useState(false)
  const [truncated, setTruncated] = useState(false)
  const [error, setError] = useState("")
  const [all, setAll] = useState(false)
  const [loading, setLoading] = useState(true)
  const src = `${url}&ref=${encodeURIComponent(a.ref)}`
  useEffect(() => {
    const controller = new AbortController()
    fetch(src + (all ? "&full=1" : ""), { signal: controller.signal }).then(async r => {
      if (!r.ok) throw new Error(await r.text())
      if (r.headers.get("Content-Type")?.startsWith("image/")) { setImage(true); return }
      if (all) { setText(await r.text()); setTruncated(false); return }
      const data = await r.json(); setText(data.text); setTruncated(data.truncated)
    }).catch(e => { if (!controller.signal.aborted) setError(String(e)) }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [src, all])
  const rendered = previewText(a.name, text, truncated)
  const btn = "inline-flex h-7 items-center gap-1 rounded-md px-2 text-xs text-muted-foreground hover:bg-accent hover:text-foreground"
  // A nested Radix dialog: the task drawer is modal, so anything outside a dialog of its own cannot be clicked.
  return <Dialog open onOpenChange={o => { if (!o) close() }}>
  <DialogContent showCloseButton={false} aria-describedby={undefined} className="attachment-viewer flex max-h-[calc(100dvh-24px)] w-[calc(100vw-24px)] max-w-4xl flex-col gap-0 overflow-hidden p-0 sm:max-w-4xl">
    <header className="flex shrink-0 items-center gap-1 border-b px-3 py-1.5">
      <DialogTitle className="mr-auto truncate text-[13px] font-semibold">{label(a)}</DialogTitle>
      {!image && <button type="button" className={btn} onClick={() => navigator.clipboard.writeText(text).catch(e => setError(String(e)))}><Copy className="size-3.5" />Copy</button>}
      <a className={btn} href={src + "&download=1"} download={a.name}><Download className="size-3.5" />Download</a>
      <button type="button" className={btn} aria-label="Close preview" onClick={close}><X className="size-4" /></button>
    </header>
    <div className="min-h-0 flex-1 overflow-auto p-3 text-[12.5px]">
    {error ? <p role="alert">{error}</p> : loading ? <p>Loading…</p> : image ? <img src={src} alt={a.name} loading="lazy" /> : type(a) === "markdown" ? <Md text={text} /> : <pre>{/\.(diff|patch)$/.test(a.name) ? rendered.split("\n").map((l, i) => <span key={i} style={{ display: "block", color: l.startsWith("+") ? "#15803d" : l.startsWith("-") ? "#b91c1c" : undefined }}>{l}{"\n"}</span>) : rendered}</pre>}
    {truncated && <p className="mt-2 text-xs text-muted-foreground">Preview truncated · {human(a.size)} <button type="button" className="underline" onClick={() => { setLoading(true); setAll(true) }}>Load all</button></p>}
    </div>
  </DialogContent></Dialog>
}
