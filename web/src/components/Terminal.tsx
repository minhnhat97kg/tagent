import "@xterm/xterm/css/xterm.css"
import { Terminal as XTerm } from "@xterm/xterm"
import { useEffect, useRef, useState } from "react"
import { post } from "@/api"
import { enc } from "@/hooks/useRoute"

interface Frame {
  screen: string
  cols: number
  rows: number
  cx: number
  cy: number
  cursor: boolean
}

// Terminal mirrors a tmux pane over SSE and sends keystrokes back.
export function Terminal({ pane }: { pane: string }) {
  const host = useRef<HTMLDivElement>(null)
  const [gone, setGone] = useState(false)

  useEffect(() => {
    const el = host.current!
    const wrap = el.parentElement!
    const term = new XTerm({
      cursorBlink: false,
      convertEol: false,
      scrollback: 0,
      allowProposedApi: true,
      fontFamily: 'ui-monospace, "SF Mono", Menlo, monospace',
      fontSize: 13,
      theme: {
        background: getComputedStyle(document.documentElement).getPropertyValue("--term-bg").trim() || "#0a0a0a",
        foreground: "#ffffff",
      },
    })
    term.open(el)

    let cols = 0
    let rows = 0
    // ponytail: scale the font to fit the width by a 0.6em advance guess; measure if it looks off.
    const fit = () => {
      if (!cols) return
      const w = wrap.clientWidth - 24
      term.options.fontSize = Math.max(6, Math.min(16, Math.floor((w / (cols * 0.6)) * 10) / 10))
    }

    const es = new EventSource(`/api/pane/stream?pane=${enc(pane)}`)
    es.onmessage = (e) => {
      const f: Frame = JSON.parse(e.data)
      if (f.cols !== cols || f.rows !== rows) {
        cols = f.cols
        rows = f.rows
        if (cols > 0 && rows > 0) term.resize(cols, rows)
        fit()
      }
      // Overwrite in place (no clear first) to avoid flicker; the last line has no newline so nothing scrolls.
      term.write(
        "\x1b[H" +
          f.screen
            .split("\n")
            .map((l) => l + "\x1b[0m\x1b[K")
            .join("\r\n") +
          "\x1b[J" +
          `\x1b[${f.cy + 1};${f.cx + 1}H` +
          (f.cursor ? "\x1b[?25h" : "\x1b[?25l"),
      )
    }
    es.addEventListener("gone", () => {
      es.close()
      setGone(true)
    })

    // Keystrokes are batched per frame and sent in order.
    let chain: Promise<unknown> = Promise.resolve()
    let buf = ""
    let raf = 0
    const disposeData = term.onData((d) => {
      buf += d
      if (!raf) {
        raf = requestAnimationFrame(() => {
          const data = buf
          buf = ""
          raf = 0
          chain = chain.then(() => post("/api/pane/input", { pane, data })).catch(() => {})
        })
      }
    })

    const ro = new ResizeObserver(fit)
    ro.observe(wrap)
    return () => {
      es.close()
      ro.disconnect()
      cancelAnimationFrame(raf)
      disposeData.dispose()
      term.dispose()
    }
  }, [pane])

  return (
    <div data-terminal className="relative min-h-0 flex-1 overflow-auto rounded-xl bg-[var(--term-bg)] [touch-action:pan-y]">
      <div ref={host} className="mx-auto w-fit p-3" />
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
