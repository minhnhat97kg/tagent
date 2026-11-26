import { useEffect, useState } from "react"

// Hash routes. Ids are encoded with enc(). Old links keep working; /pane/<id> opens one pane maximized.
// /agent/<pane> and /agents/<pane> open home with that agent selected.
const read = () => location.hash.slice(1) || "/"

export const enc = encodeURIComponent
export const nav = (path: string) => {
  location.hash = path
}

export type Route =
  | { page: "home"; agent?: string } // agent: the pane id of /agent/<pane>, selected in the chat pane
  | { page: "routing" | "boardConfig" }
  | { page: "pane"; pane: "usage" | "tasks" | "chat" }
  | { page: "agents" } // the Agents definitions page
  | { page: "custom"; name?: string } // a custom pane (#/custom/<name>); no name = the first one
  | { page: "flows"; name: string } // "" = no workflow chosen
  | { page: "run"; ticket: string; dir: string | null }

// parseRoute reads a hash route. Anything unknown, or an id that will not decode, is home.
export function parseRoute(route: string): Route {
  const q = route.indexOf("?")
  const path = q < 0 ? route : route.slice(0, q)
  const dir = q < 0 ? null : new URLSearchParams(route.slice(q + 1)).get("dir")
  const parts: string[] = []
  for (const s of path.split("/").slice(1)) {
    try { parts.push(decodeURIComponent(s)) } catch { return { page: "home" } }
  }
  const [a, b, c] = parts
  switch (a) {
    case "routing": return parts.length === 1 ? { page: "routing" } : { page: "home" }
    case "board": return b === "config" && parts.length === 2 ? { page: "boardConfig" } : { page: "home" }
    case "pane":
      if (b === "usage" || b === "tasks" || b === "chat") return { page: "pane", pane: b }
      if (b === "agents") return { page: "agents" }
      if (b === "workflows") return { page: "flows", name: "" }
      return { page: "home" }
    case "agent": return b ? { page: "home", agent: b } : { page: "home" }
    case "agents":
      if (parts.length === 1) return { page: "agents" }
      return b && parts.length === 2 ? { page: "home", agent: b } : { page: "home" }
    case "workflows":
      if (c) return { page: "run", ticket: c, dir }
      return { page: "flows", name: b ?? "" }
    case "runs": return b ? { page: "run", ticket: b, dir } : { page: "home" }
    case "custom": return parts.length === 1 ? { page: "custom" } : b && parts.length === 2 ? { page: "custom", name: b } : { page: "home" }
    default: return { page: "home" }
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
