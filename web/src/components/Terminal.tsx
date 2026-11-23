import "@xterm/xterm/css/xterm.css"
import { Terminal as XTerm } from "@xterm/xterm"
import { WebglAddon } from "@xterm/addon-webgl"
import { useEffect, useRef, useState } from "react"
import { paneInput, registerPaneSocket } from "@/lib/paneInput"
import { toast } from "sonner"
import { enc } from "@/hooks/useRoute"

interface Frame {
  screen: string
  cols: number
  rows: number
  cx: number
  cy: number
  cursor: boolean
}

const touch = matchMedia("(pointer: coarse)").matches
const MIN_READABLE = touch ? 11 : 9

// typedDiff turns a text box edit into terminal keys: DEL for each removed code point, then the new text.
// Diffing whole values handles iOS IMEs (Vietnamese Telex/VNI), autocorrect and suggestions alike.
export function typedDiff(before: string, after: string) {
  const a = Array.from(before), b = Array.from(after)
  let p = 0
  while (p < a.length && p < b.length && a[p] === b[p]) p++
  return "\x7f".repeat(a.length - p) + b.slice(p).join("")
}

// Terminal mirrors a tmux pane over WebSocket and sends keystrokes back.
export function Terminal({ pane, readOnly = false, onConnection }: { pane: string; label?: string; readOnly?: boolean; onConnection?: (connected: boolean) => void }) {
  const host = useRef<HTMLDivElement>(null)
  const [gone, setGone] = useState(false)

  useEffect(() => {
    const el = host.current!
    const wrap = el.parentElement!
    const term = new XTerm({
      cursorBlink: false,
      disableStdin: readOnly,
      convertEol: false,
      scrollback: 0,
      allowProposedApi: true,
      fontFamily: '"JetBrains Mono", ui-monospace, "SF Mono", Menlo, monospace',
      lineHeight: 1.4,
      fontSize: 13,
      // Light theme matching the web panels (paper/ink); contrast floor keeps dark-themed truecolor output readable.
      minimumContrastRatio: 4.5,
      theme: {
        background: "#ffffff",
        foreground: "#18181b",
        cursor: "#18181b",
        cursorAccent: "#ffffff",
        selectionBackground: "#e4e4e7",
        black: "#18181b", red: "#dc2626", green: "#15803d", yellow: "#a16207",
        blue: "#1d4ed8", magenta: "#be185d", cyan: "#0e7490", white: "#71717a",
        brightBlack: "#52525b", brightRed: "#ef4444", brightGreen: "#16a34a", brightYellow: "#ca8a04",
        brightBlue: "#2563eb", brightMagenta: "#db2777", brightCyan: "#0891b2", brightWhite: "#3f3f46",
      },
    })
    term.open(el)
    // GPU renderer; falls back to the DOM renderer when WebGL is missing or its context is lost.
    try {
      const gl = new WebglAddon()
      gl.onContextLoss(() => gl.dispose())
      term.loadAddon(gl)
    } catch { /* DOM renderer */ }
    if (readOnly && term.textarea) { term.textarea.tabIndex = -1; term.textarea.disabled = true }
    term.attachCustomKeyEventHandler(e => !(e.key === "F6" && e.ctrlKey && e.shiftKey))

    let cols = 0
    let rows = 0
    let rowRatio = 0 // drawn row height per font px, learned after a render (xterm rounds rows to whole pixels)
    const fit = () => {
      if (!cols || !wrap.clientWidth) return
      const padding = getComputedStyle(el)
      const w = wrap.clientWidth - parseFloat(padding.paddingLeft) - parseFloat(padding.paddingRight)
      const measure = document.createElement("span")
      measure.textContent = "M"
      Object.assign(measure.style, {
        position: "absolute", visibility: "hidden", whiteSpace: "pre",
        fontFamily: '"JetBrains Mono", ui-monospace, "SF Mono", Menlo, monospace',
        fontSize: "16px", lineHeight: "1.4",
      })
      el.appendChild(measure)
      const { width: cellWidth, height: cellHeight } = measure.getBoundingClientRect()
      measure.remove()
      if (!cellWidth || !cellHeight) return
      const h = wrap.clientHeight - parseFloat(padding.paddingTop) - parseFloat(padding.paddingBottom)
      // The whole remote screen fits the pane: the smaller of the width and height fits, no scrolling either way.
      const byWidth = 16 * w * 0.985 / (cellWidth * cols)
      const byHeight = rows && h > 0 ? h / (rows * (rowRatio || cellHeight / 16)) : byWidth
      // Fit the whole screen, but never below a readable size: then fit the width and scroll to the cursor.
      const size = Math.max(6, Math.min(20, Math.max(Math.min(byWidth, byHeight), Math.min(byWidth, MIN_READABLE))))
      term.options.fontSize = Math.floor(size * 4) / 4
      requestAnimationFrame(() => {
        const screen = el.querySelector<HTMLElement>(".xterm-screen")
        if (!screen || !rows) return
        const learned = screen.offsetHeight / rows / term.options.fontSize!
        if (Math.abs(learned - rowRatio) > 0.01) { rowRatio = learned; fit() }
      })
    }

    let ws: WebSocket
    let stopped = false
    let retry = 250
    let timer: ReturnType<typeof setTimeout> | undefined
    let pending: { resolve: () => void; reject: (e: Error) => void } | undefined
    const unregister = registerPaneSocket(pane, data => new Promise<void>((resolve, reject) => {
      if (ws?.readyState !== WebSocket.OPEN) { reject(new Error("Terminal disconnected")); return }
      pending = { resolve, reject }
      ws.send(new TextEncoder().encode(data))
    }))
    const connect = () => {
      ws = new WebSocket(`${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/api/pane/ws?pane=${enc(pane)}`)
      ws.binaryType = "arraybuffer"
      ws.onopen = () => { setGone(false); retry = 250 }
      ws.onmessage = e => {
        if (e.data instanceof ArrayBuffer) { term.write(new Uint8Array(e.data)); return }
        const f: Frame & { type: string; ok?: boolean } = JSON.parse(e.data)
        if (f.type === "ack") {
          if (f.ok) pending?.resolve()
          else pending?.reject(new Error("Terminal input failed; delivery uncertain"))
          pending = undefined
          return
        }
        if (f.type === "gone") { stopped = true; setGone(true); ws.close(); return }
        if (f.cols !== cols || f.rows !== rows) {
          cols = f.cols; rows = f.rows
          if (cols > 0 && rows > 0) term.resize(cols, rows)
          fit()
        }
        if (f.type === "snapshot") {
          term.reset()
          term.write("\x1b[H" + f.screen.split("\n").map(l => l + "\x1b[0m\x1b[K").join("\r\n") +
            "\x1b[J" + `\x1b[${f.cy + 1};${f.cx + 1}H` + (f.cursor ? "\x1b[?25h" : "\x1b[?25l"))
          term.options.disableStdin = readOnly
          onConnection?.(true)
        }
      }
      ws.onclose = () => {
        term.options.disableStdin = true
        onConnection?.(false)
        pending?.reject(new Error("Terminal disconnected; delivery uncertain, not retried")); pending = undefined
        if (!stopped) { timer = setTimeout(connect, retry); retry = Math.min(retry * 2, 10000) }
      }
      ws.onerror = () => ws.close()
    }
    onConnection?.(false)
    connect()

    // Queue immediately so a later approval cannot overtake buffered keystrokes.
    const disposeData = term.onData((data) => {
      if (readOnly || ws.readyState !== WebSocket.OPEN) return
      void paneInput(pane, "input", data).catch(e => toast.error(e.message))
    })

    // Touch: xterm's iOS IME handling drops Vietnamese marks, so typing goes through our own box and is diffed.
    let box: HTMLTextAreaElement | undefined
    if (touch && !readOnly) {
      if (term.textarea) term.textarea.readOnly = true
      box = document.createElement("textarea")
      box.setAttribute("aria-label", "Type into terminal")
      box.autocapitalize = "off"
      box.spellcheck = false
      Object.assign(box.style, { position: "absolute", left: "0", bottom: "0", width: "100%", height: "32px", fontSize: "16px", opacity: "0", border: "0", resize: "none" })
      wrap.parentElement!.appendChild(box)
      let last = ""
      const sendKeys = (data: string) => { if (data && ws.readyState === WebSocket.OPEN) void paneInput(pane, "input", data).catch(e => toast.error(e.message)) }
      box.addEventListener("input", () => { sendKeys(typedDiff(last, box!.value)); last = box!.value })
      box.addEventListener("keydown", e => {
        if (e.isComposing) return
        if (e.key === "Enter") { e.preventDefault(); sendKeys("\r"); box!.value = last = "" }
        else if (e.key === "Backspace" && !box!.value) { e.preventDefault(); sendKeys("\x7f") }
      })
      wrap.addEventListener("click", () => box!.focus({ preventScroll: true }))
    }

    const ro = new ResizeObserver(fit)
    ro.observe(wrap)
    // The first frame may arrive before the web font replaces the fallback.
    let disposed = false
    document.fonts.ready.then(() => {
      if (!disposed) fit()
    })
    document.fonts.addEventListener("loadingdone", fit)
    return () => {
      disposed = true
      document.fonts.removeEventListener("loadingdone", fit)
      stopped = true
      clearTimeout(timer)
      unregister()
      ws.close()
      ro.disconnect()
      disposeData.dispose()
      box?.remove()
      term.dispose()
    }
  }, [pane, readOnly, onConnection])

  return (
    <div data-terminal data-readonly={readOnly || undefined} className="relative flex min-h-0 flex-1 flex-col overflow-hidden bg-white text-zinc-800">
      <div className="min-h-0 min-w-0 flex-1 overflow-x-hidden overflow-y-auto [touch-action:pan-y]"><div ref={host} className="w-fit p-1 md:p-2" /></div>
      {gone && (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-[var(--term-bg)]/90 text-[var(--term-fg)]">
          <p>Pane closed</p>
          <a href="#/" className="underline">
            Home
          </a>
        </div>
      )}
    </div>
  )
}
