import { useLayoutEffect, useRef } from "react"
import { extReply } from "@/lib/extBridge"

// ExtPane shows one custom pane. The iframe has no allow-same-origin, so it cannot read the app DOM,
// localStorage or the access cookie; it calls /api/* through the bridge in lib/extBridge.ts.
export function ExtPane({ name }: { name: string }) {
  const ref = useRef<HTMLIFrameElement>(null)
  useLayoutEffect(() => {
    const onMessage = (e: MessageEvent) => {
      const frame = ref.current?.contentWindow ?? null
      extReply(e, frame)?.then(r => frame?.postMessage(r, "*")) // the pane's origin is opaque, so "*"
    }
    window.addEventListener("message", onMessage)
    return () => window.removeEventListener("message", onMessage)
  }, [])
  return <iframe ref={ref} sandbox="allow-scripts" src={`/ext/${encodeURIComponent(name)}.html`} title={name} className="min-h-0 w-full flex-1 border-0 bg-background" />
}
