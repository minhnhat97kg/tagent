import "@xterm/xterm/css/xterm.css"
import { Terminal as XTerm } from "@xterm/xterm"
import { useEffect, useRef, useState, type RefObject } from "react"
import { paneInput, registerPaneSocket } from "@/lib/paneInput"
import { toast } from "sonner"
import { enc } from "@/hooks/useRoute"
import { bindTerminalTap } from '@/lib/touchViewport'
import { bindTerminalSwipe, sgrWheel, snapshotBytes } from '@/lib/terminalScroll'
import { terminalFontSize, type TerminalControl, type TerminalOwnership } from '@/lib/terminalFit'

interface Frame {
  screen: string
  cols: number
  rows: number
  cx: number
  cy: number
  cursor: boolean
  resizable: boolean
  alternate: boolean
  mouse?: boolean
}

// One id per device (browser), so Handoff survives reloads and reconnects: the latest Handoff wins.
function viewerId() {
  try {
    let id = localStorage.getItem("tagent.viewer")
    if (!id) { id = Array.from(crypto.getRandomValues(new Uint8Array(12)), b => b.toString(16).padStart(2, "0")).join(""); localStorage.setItem("tagent.viewer", id) } // randomUUID needs https; the Tailscale URL is http
    return id
  } catch { return (window as unknown as { __tagentViewer?: string }).__tagentViewer ??= Math.random().toString(36).slice(2, 14) }
}

const touch = matchMedia("(pointer: coarse)").matches
const MIN_READABLE = 9 // below this, fit the width and scroll vertically instead of shrinking further

// Terminal palettes matching the web's light and dark tokens (index.css).
const LIGHT = {
  background: "#ffffff", foreground: "#18181b", cursor: "#18181b", cursorAccent: "#ffffff", selectionBackground: "#e4e4e7",
  black: "#18181b", red: "#dc2626", green: "#15803d", yellow: "#a16207", blue: "#1d4ed8", magenta: "#be185d", cyan: "#0e7490", white: "#71717a",
  brightBlack: "#52525b", brightRed: "#ef4444", brightGreen: "#16a34a", brightYellow: "#ca8a04", brightBlue: "#2563eb", brightMagenta: "#db2777", brightCyan: "#0891b2", brightWhite: "#3f3f46",
}
const DARK = {
  background: "#18181b", foreground: "#e4e4e7", cursor: "#e4e4e7", cursorAccent: "#18181b", selectionBackground: "#3f3f46",
  black: "#27272a", red: "#f87171", green: "#4ade80", yellow: "#facc15", blue: "#60a5fa", magenta: "#f472b6", cyan: "#22d3ee", white: "#e4e4e7",
  brightBlack: "#71717a", brightRed: "#fca5a5", brightGreen: "#86efac", brightYellow: "#fde047", brightBlue: "#93c5fd", brightMagenta: "#f9a8d4", brightCyan: "#67e8f9", brightWhite: "#fafafa",
}
const termTheme = () => (document.documentElement.classList.contains("dark") ? DARK : LIGHT)

// typedDiff turns a text box edit into terminal keys: DEL for each removed code point, then the new text.
// Diffing whole values handles iOS IMEs (Vietnamese Telex/VNI), autocorrect and suggestions alike.
export function typedDiff(before: string, after: string) {
  const a = Array.from(before), b = Array.from(after)
  let p = 0
  while (p < a.length && p < b.length && a[p] === b[p]) p++
  return "\x7f".repeat(a.length - p) + b.slice(p).join("")
}

// Terminal mirrors a tmux pane over WebSocket and sends keystrokes back.
export function Terminal({ pane, onConnection, keyboardInputRef, onKeyboardChange, controlRef, onOwnership }: { controlRef?: RefObject<TerminalControl | null>; onOwnership?: (state: TerminalOwnership) => void; pane: string; label?: string; onConnection?: (connected: boolean) => void; keyboardInputRef?: RefObject<HTMLTextAreaElement | null>; onKeyboardChange?: (open: boolean) => void }) {
  const host = useRef<HTMLDivElement>(null)
  const [gone, setGone] = useState(false)

  useEffect(() => {
    const el = host.current!
    const wrap = el.parentElement!
    const term = new XTerm({
      cursorBlink: false,
      disableStdin: true,
      convertEol: false,
      scrollback: 5000,
      smoothScrollDuration: touch ? 0 : 90, // eased wheel scrolling through history (touch has its own momentum)
      scrollOnUserInput: false,
      allowProposedApi: true,
      fontFamily: '"JetBrains Mono", ui-monospace, "SF Mono", Menlo, monospace',
      lineHeight: 1.4,
      fontSize: terminalFontSize(window.innerWidth),
      // Light theme matching the web panels (paper/ink); contrast floor keeps dark-themed truecolor output readable.
      minimumContrastRatio: 4.5,
      theme: termTheme(),
    })
    term.open(el)
    const themeWatch = new MutationObserver(() => { term.options.theme = termTheme() })
    themeWatch.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] })
    term.attachCustomKeyEventHandler(e => !(e.key === "F6" && e.ctrlKey && e.shiftKey))
    // Full-screen apps (alternate screen) scroll themselves: forward the wheel as SGR mouse events when the app
    // reads the mouse, never let xterm turn it into arrow keys. The normal screen scrolls xterm's own history.
    let appMouse = false
    let wheelPixels = 0
    // Batch wheel/swipe into one send per animation frame: fewer round trips, steadier app redraws.
    let wheelFrame = 0
    const forwardWheel = (pixels: number) => {
      wheelPixels += pixels
      if (wheelFrame) return
      wheelFrame = requestAnimationFrame(() => {
        wheelFrame = 0
        const step = Math.max(8, cellPixels)
        const steps = Math.trunc(wheelPixels / step)
        if (!steps) return
        wheelPixels -= steps * step
        void paneInput(pane, "input", sgrWheel(steps < 0, Math.min(10, Math.abs(steps)), Math.ceil(term.cols / 2), Math.ceil(term.rows / 2))).catch(() => {})
      })
    }
    term.attachCustomWheelEventHandler(e => {
      if (term.buffer.active.type !== 'alternate') return true
      if (appMouse) { e.preventDefault(); forwardWheel(e.deltaMode === 1 ? e.deltaY * cellPixels : e.deltaY) }
      return false
    })

    let cols = 0
    let rows = 0
    let resizable = false
    let automatic = false
    let resizeTimer: ReturnType<typeof setTimeout> | undefined
    let desiredSize = { cols: 40, rows: 10 }
    let cellPixels = 18
    let rowRatio = 0 // real drawn row height per font px (xterm uses char height x lineHeight, rounded), learned after render
    let swipePixels = 0
    const unbindSwipe = touch ? bindTerminalSwipe(wrap, pixels => {
      if (term.buffer.active.type === 'alternate') { swipePixels = 0; if (appMouse) forwardWheel(pixels); return }
      swipePixels += pixels
      const lines = Math.trunc(swipePixels / cellPixels)
      if (lines) { term.scrollLines(lines); swipePixels -= lines * cellPixels }
    }) : undefined
    // xterm owns both the visible rows and history; the outer shell never scrolls.
    const fitRows = (height: number) => {
      const localRows = Math.max(1, Math.floor(height / cellPixels))
      if (cols > 0 && (term.cols !== cols || term.rows !== localRows)) term.resize(cols, localRows)
    }
    // Measure a drawn row once the renderer has applied the font; if the estimate was off, fit again so the
    // last rows (the agent's input line) never fall below the pane.
    const learnRows = () => requestAnimationFrame(() => requestAnimationFrame(() => {
      const row = el.querySelector<HTMLElement>(".xterm-rows > div")
      const font = term.options.fontSize!
      if (!row || !row.offsetHeight || !font) return
      const learned = row.offsetHeight / font
      if (Math.abs(learned - rowRatio) > 0.01) { rowRatio = learned; fit() }
    }))
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
      const baseFont = terminalFontSize(window.innerWidth)
      const desiredCols = Math.max(40, Math.min(300, Math.floor(w / (cellWidth * baseFont / 16))))
      const rowH = (font: number) => font * (rowRatio || cellHeight / 16)
      const desiredRows = Math.max(10, Math.min(120, Math.floor(h / rowH(baseFont))))
      desiredSize = { cols: desiredCols, rows: desiredRows }
      // Only the owner may apply these; viewers still report their current geometry.
      clearTimeout(resizeTimer)
      resizeTimer = setTimeout(() => {
        if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'resize', cols: desiredCols, rows: desiredRows }))
      }, 150)
      if (resizable) {
        term.options.fontSize = baseFont
        cellPixels = rowH(baseFont)
        fitRows(h)
        learnRows()
        return
      }
      // A real terminal owns this pane's geometry: fit width, floor 9px, scroll vertically.
      const byWidth = 16 * w * 0.985 / (cellWidth * cols)
      // Never shrink below 9px for a pane whose size belongs to a real terminal.
      const size = Math.max(MIN_READABLE, Math.min(baseFont, byWidth))
      if (!Number.isFinite(size)) return
      term.options.fontSize = Math.floor(size * 4) / 4
      cellPixels = rowH(term.options.fontSize)
      fitRows(h)
      // Account for xterm's cell rounding without crossing the 9px floor.
      const settle = (n = 0) => requestAnimationFrame(() => requestAnimationFrame(() => {
        if (resizable) return
        const size = term.options.fontSize!
        const tooWide = cols * cellWidth * size / 16 > w
        if (n < 24 && size > MIN_READABLE && tooWide) { term.options.fontSize = size - 0.25; settle(n + 1) }
        else { cellPixels = rowH(size); fitRows(h); learnRows() }
      }))
      settle()
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
      ws = new WebSocket(`${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/api/pane/ws?pane=${enc(pane)}&viewer=${viewerId()}`)
      ws.binaryType = "arraybuffer"
      ws.onopen = () => { setGone(false); retry = 250 }
      ws.onmessage = e => {
        if (e.data instanceof ArrayBuffer) { term.write(new Uint8Array(e.data)); return }
        const f: Frame & { type: string; ok?: boolean; owner?: boolean; message?: string; automatic?: boolean } = JSON.parse(e.data)
        if (typeof f.mouse === "boolean") appMouse = f.mouse
        if (f.type === 'error') { toast.error(f.message ?? 'Terminal sizing failed'); return }
        if (f.type === 'ownership') {
          automatic = !!f.automatic
          const changed = resizable !== !!f.owner
          resizable = !!f.owner
          onOwnership?.({ owner: resizable, cols: f.cols, rows: f.rows })
          if (changed) fit()
          return
        }
        if (f.type === "ack") {
          if (f.ok) pending?.resolve()
          else pending?.reject(new Error("Terminal input failed; delivery uncertain"))
          pending = undefined
          return
        }
        if (f.type === "gone") { stopped = true; setGone(true); ws.close(); return }
        const modeChanged = resizable !== !!f.resizable
        if (modeChanged) { resizable = !!f.resizable; clearTimeout(resizeTimer) }
        if (f.cols !== cols || f.rows !== rows || modeChanged) {
          cols = f.cols; rows = f.rows
          fit()
        }
        if (f.type === "snapshot") {
          term.reset()
          // Capture includes history followed by the remote visible rows. Writing it
          // sequentially seeds xterm's history, rather than painting only its screen.
          term.write(snapshotBytes(f.screen, f.rows, term.rows, f.cx, f.cy, f.cursor, f.alternate))
          term.options.disableStdin = false
          fit() // Reassert the browser's sizing vote after reconnect, even if geometry is unchanged.
          onOwnership?.({ owner: resizable, cols: f.cols, rows: f.rows })
          automatic = !!f.automatic
          if (automatic && !document.hidden) sendControl('handoff')
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
    const sendControl = (type: 'handoff' | 'release') => {
      clearTimeout(resizeTimer)
      if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type, ...desiredSize }))
    }
    if (controlRef) controlRef.current = { handoff: () => sendControl('handoff'), release: () => sendControl('release') }
    const activate = () => { if (automatic && !document.hidden) sendControl('handoff') }
    const pagehide = () => { sendControl('release'); ws?.close() }
    window.addEventListener('focus', activate)
    window.addEventListener('pagehide', pagehide)
    document.addEventListener('visibilitychange', activate)
    onConnection?.(false)
    connect()

    // Queue immediately so a later approval cannot overtake buffered keystrokes.
    const disposeData = term.onData((data) => {
      if (ws.readyState !== WebSocket.OPEN) return
      void paneInput(pane, "input", data).catch(e => toast.error(e.message))
    })

    // Touch: xterm's iOS IME handling drops Vietnamese marks, so typing goes through our own box and is diffed.
    let box: HTMLTextAreaElement | undefined
    let unbindTap: (() => void) | undefined
    if (touch) {
      if (term.textarea) term.textarea.readOnly = true
      box = document.createElement("textarea")
      box.setAttribute("aria-label", "Type into terminal")
      box.autocapitalize = "off"
      box.spellcheck = false
      Object.assign(box.style, { position: "absolute", left: "0", top: "0", width: "1px", height: "1px", fontSize: "16px", opacity: "0", border: "0", padding: "0", resize: "none", pointerEvents: "none" })
      wrap.parentElement!.appendChild(box)
      let last = ""
      const sendKeys = (data: string) => { if (data && ws.readyState === WebSocket.OPEN) void paneInput(pane, "input", data).catch(e => toast.error(e.message)) }
      box.setAttribute("autocorrect", "off")
      box.setAttribute("autocomplete", "off")
      // The diff assumes typing at the end; keep the caret there and start each word fresh so
      // stale text or an iOS caret move can't make whole runs be retyped.
      const toEnd = () => { const n = box!.value.length; if (box!.selectionStart !== n) box!.setSelectionRange(n, n) }
      box.addEventListener("select", toEnd)
      box.addEventListener("input", e => {
        sendKeys(typedDiff(last, box!.value)); last = box!.value
        if (!(e as InputEvent).isComposing && /\s$/.test(last)) box!.value = last = ""
        toEnd()
      })
      box.addEventListener("keydown", e => {
        if (e.isComposing) return
        if (e.key === "Enter") { e.preventDefault(); sendKeys("\r"); box!.value = last = "" }
        else if (e.key === "Backspace" && !box!.value) { e.preventDefault(); sendKeys("\x7f") }
      })
      // A genuine tap may type; native scrolls/drags must never open the keyboard.
      unbindTap = bindTerminalTap(wrap, () => box!.focus({ preventScroll: true }))
      if (keyboardInputRef) keyboardInputRef.current = box
      box.addEventListener("focus", () => onKeyboardChange?.(true))
      box.addEventListener("blur", () => onKeyboardChange?.(false))
    }

    const ro = new ResizeObserver(fit)
    ro.observe(wrap)
    window.addEventListener('resize', fit)
    window.visualViewport?.addEventListener('resize', fit)
    // The first frame may arrive before the web font replaces the fallback.
    let disposed = false
    document.fonts.ready.then(() => {
      if (!disposed) fit()
    })
    document.fonts.addEventListener("loadingdone", fit)
    return () => {
      disposed = true
      if (controlRef) controlRef.current = null
      window.removeEventListener('focus', activate)
      window.removeEventListener('pagehide', pagehide)
      document.removeEventListener('visibilitychange', activate)
      window.removeEventListener('resize', fit)
      window.visualViewport?.removeEventListener('resize', fit)
      document.fonts.removeEventListener("loadingdone", fit)
      stopped = true
      clearTimeout(timer)
      clearTimeout(resizeTimer)
      unregister()
      ws.close()
      ro.disconnect()
      disposeData.dispose()
      unbindTap?.()
      unbindSwipe?.()
      box?.blur()
      if (keyboardInputRef && keyboardInputRef.current === box) keyboardInputRef.current = null
      box?.remove()
      themeWatch.disconnect()
      term.dispose()
    }
  }, [pane, onConnection, keyboardInputRef, onKeyboardChange, controlRef, onOwnership])

  return (
    <div data-terminal className="relative flex min-h-0 flex-1 flex-col overflow-hidden bg-card text-card-foreground">
      <div className="min-h-0 min-w-0 flex-1 overflow-hidden overscroll-contain [touch-action:none]"><div ref={host} className="h-full w-full overflow-hidden p-1 md:p-2" /></div>
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
