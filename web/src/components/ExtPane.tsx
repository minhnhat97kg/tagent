import { useLayoutEffect, useRef } from "react"
import { api } from "@/api"
import { extReply } from "@/lib/extBridge"
import type { ExtInfo } from "@/lib/ext"

// ExtPane shows one custom pane. The iframe has no allow-same-origin, so it cannot read the app DOM,
// localStorage or the access cookie; it calls /api/* through the bridge in lib/extBridge.ts, which asks the
// server for the pane's access at each write, so a changed pane loses a grant at once.
export function ExtPane({ name }: { name: string }) {
  const ref = useRef<HTMLIFrameElement>(null)
  useLayoutEffect(() => {
    const canAct = () => api<ExtInfo[]>("/api/ext").then(l => { const p = l.find(x => x.name === name); return !!p && (!p.agent || p.actions) })
    const onMessage = (e: MessageEvent) => {
      const frame = ref.current?.contentWindow ?? null
      extReply(e, frame, name, canAct)?.then(r => frame?.postMessage(r, "*")) // the pane's origin is opaque, so "*"
    }
    window.addEventListener("message", onMessage)
    return () => window.removeEventListener("message", onMessage)
  }, [name])
  return <iframe ref={ref} sandbox="allow-scripts" src={`/ext/${encodeURIComponent(name)}.html`} title={name} className="min-h-0 w-full flex-1 border-0 bg-background" />
}
