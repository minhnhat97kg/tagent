import { useEffect, useRef } from "react"
import { enc, nav } from "@/hooks/useRoute"
import type { Agent } from "@/types"

// Keys do nothing while typing, inside the pane's terminal, or in a dialog.
const SKIP = "input, textarea, select, [contenteditable], [data-terminal], [role=dialog]"

// useKeys binds the dashboard shortcuts: j/k move, n jumps to the next agent that needs you,
// Enter focuses the terminal, ? opens help. Ctrl+Shift+F6 returns focus to the agent header.
export function useKeys(order: Agent[], selPane: string | undefined, onHelp: () => void) {
  const latest = useRef({ order, selPane, onHelp })
  latest.current = { order, selPane, onHelp }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null
      if (e.key === "F6" && e.ctrlKey && e.shiftKey && t?.closest?.("[data-terminal]")) {
        e.stopPropagation()
        e.preventDefault()
        t.closest("[data-terminal]")?.parentElement?.querySelector<HTMLElement>("[data-agent-header]")?.focus()
        return
      }
      if (e.metaKey || e.ctrlKey || e.altKey || t?.closest?.(SKIP)) return
      const { order, selPane, onHelp } = latest.current
      const n = order.length
      const i = order.findIndex((a) => a.pane === selPane)
      const go = (idx: number) => nav("/agent/" + enc(order[(idx + n) % n]!.pane))
      switch (e.key) {
        case "j":
        case "ArrowDown":
          if (!n) return
          e.preventDefault()
          go(i + 1)
          return
        case "k":
        case "ArrowUp":
          if (!n) return
          e.preventDefault()
          go(i < 0 ? n - 1 : i - 1)
          return
        case "n": {
          const next = order.map((a, idx) => ({ a, idx })).filter((x) => x.a.status === "waiting")
          const pick = next.find((x) => x.idx > i) ?? next[0]
          if (pick) nav("/agent/" + enc(pick.a.pane))
          return
        }
        case "Enter":
          // Only when focus is on the page itself, not on a button or link that Enter should activate.
          if (!selPane || (t && t !== document.body && t.closest?.("button, a, [role=button]"))) return
          e.preventDefault()
          document.querySelector<HTMLElement>("[data-terminal]:not([data-readonly]) .xterm-helper-textarea")?.focus()
          return
        case "?":
          onHelp()
          return
      }
    }
    window.addEventListener("keydown", onKey, true)
    return () => window.removeEventListener("keydown", onKey, true)
  }, [])
}
