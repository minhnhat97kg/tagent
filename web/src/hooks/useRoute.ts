import { useEffect, useState } from "react"

// Hash routes: "/" or "/agent/<pane>". Ids are encoded with enc().
const read = () => location.hash.slice(1) || "/"

export const enc = encodeURIComponent
export const nav = (path: string) => {
  location.hash = path
}

export type Sel = { kind: "agent"; pane: string } | null

export function parseSel(route: string): Sel {
  const m = /^\/(agent)\/(.+)$/.exec(route)
  if (!m) return null
  try {
    const v = decodeURIComponent(m[2]!)
    return { kind: "agent", pane: v }
  } catch {
    return null
  }
}

export function useRoute() {
  const [route, setRoute] = useState(read)
  useEffect(() => {
    const f = () => setRoute(read())
    window.addEventListener("hashchange", f)
    return () => window.removeEventListener("hashchange", f)
  }, [])
  return route
}
