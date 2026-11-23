import { useState } from "react"
import { useAppState } from "@/state"

const KEY = "ta.project"

// useProjectDir is the project on screen: the selected agent's project if it has one,
// else the remembered choice, else the first project.
export function useProjectDir(agentPath?: string): [string, (dir: string) => void] {
  const { state } = useAppState()
  const projects = state?.projects ?? []
  const [sel, setSel] = useState(() => {
    try {
      return localStorage.getItem(KEY) ?? ""
    } catch {
      return ""
    }
  })
  const owner = agentPath
    ? projects
        .filter((p) => agentPath === p || agentPath.startsWith(p + "/"))
        .sort((a, b) => b.length - a.length)[0]
    : undefined
  const dir = owner ?? (projects.includes(sel) ? sel : (projects[0] ?? ""))
  const set = (d: string) => {
    setSel(d)
    try {
      localStorage.setItem(KEY, d)
    } catch {
      // storage blocked: the choice lasts this page view only
    }
  }
  return [dir, set]
}
