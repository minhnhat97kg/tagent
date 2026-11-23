import { useEffect, useState } from "react"

// The web theme belongs to this browser only. It never reads or writes the
// TUI theme (@agent_theme): "system" follows the OS, light/dark pin it.
export type WebTheme = "system" | "light" | "dark"
const KEY = "tagent.theme"
export const nextTheme: Record<WebTheme, WebTheme> = { system: "light", light: "dark", dark: "system" }

function load(): WebTheme {
  try {
    const v = localStorage.getItem(KEY)
    if (v === "light" || v === "dark") return v
  } catch { /* storage blocked: follow the OS */ }
  return "system"
}

export function useWebTheme(): [WebTheme, (t: WebTheme) => void] {
  const [theme, setTheme] = useState<WebTheme>(load)
  useEffect(() => {
    const mq = window.matchMedia?.("(prefers-color-scheme: dark)")
    const apply = () => document.documentElement.classList.toggle("dark", theme === "dark" || (theme === "system" && !!mq?.matches))
    apply()
    mq?.addEventListener?.("change", apply)
    return () => mq?.removeEventListener?.("change", apply)
  }, [theme])
  const set = (t: WebTheme) => {
    setTheme(t)
    try { localStorage.setItem(KEY, t) } catch { /* not remembered */ }
  }
  return [theme, set]
}
