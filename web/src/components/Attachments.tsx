import { useEffect, useState } from "react"
import { previewText } from "@/lib/attachments"
import { Md } from "./BoardDrawer"

export type Attachment = { ref: string; kind: string; name: string; size: number; author: string; time: string; commentId?: string; error?: string }
export function AttachmentRows({ items, open }: { items: Attachment[]; open: (a: Attachment) => void }) {
  return <ul>{items.map(a => <li key={a.ref}><button type="button" disabled={!!a.error} title={a.error} onClick={() => open(a)}>{({ image: "▧", markdown: "¶", diff: "±", json: "{}" } as Record<string, string>)[a.kind] ?? "📎"} {a.name} · {a.size.toLocaleString()} bytes · {a.author} · {new Date(a.time).toLocaleString()}{a.error && " (unavailable)"}</button></li>)}</ul>
}
export function AttachmentViewer({ a, url, close }: { a: Attachment; url: string; close: () => void }) {
  const [text, setText] = useState("")
  const [image, setImage] = useState(false)
  const [truncated, setTruncated] = useState(false)
  const [error, setError] = useState("")
  const [full, setFull] = useState(false)
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
  return <section className={full ? "attachment-viewer attachment-full" : "attachment-viewer"} aria-label={`Preview ${a.name}`}>
    <h3>{a.name}</h3><button type="button" onClick={close}>Close preview</button><button type="button" onClick={() => setFull(!full)}>{full ? "Exit full screen" : "Open full screen"}</button>
    {!image && <button type="button" onClick={() => navigator.clipboard.writeText(text).catch(e => setError(String(e)))}>Copy</button>} <a href={src + "&download=1"} download={a.name}>Download</a>
    {error ? <p role="alert">{error}</p> : loading ? <p>Loading…</p> : image ? <img src={src} alt={a.name} loading="lazy" /> : a.kind === "markdown" ? <Md text={text} /> : <pre>{/\.(diff|patch)$/.test(a.name) ? rendered.split("\n").map((l, i) => <span key={i} style={{ display: "block", color: l.startsWith("+") ? "#15803d" : l.startsWith("-") ? "#b91c1c" : undefined }}>{l}{"\n"}</span>) : rendered}</pre>}
    {truncated && <p>Preview truncated · {a.size} bytes <button type="button" onClick={() => { setLoading(true); setAll(true) }}>Load all</button></p>}
  </section>
}
