import { createContext, useContext, type ReactNode } from "react"
import { api } from "@/api"
import { usePoll } from "@/hooks/usePoll"
import type { State } from "@/types"

interface AppState {
  state?: State
  error?: Error
}

const Ctx = createContext<AppState>({})

// One poll of /api/state shared by the whole page.
export function StateProvider({ children }: { children: ReactNode }) {
  const { data, error } = usePoll(() => api<State>("/api/state"), 1500)
  return <Ctx.Provider value={{ state: data, error }}>{children}</Ctx.Provider>
}

export const useAppState = () => useContext(Ctx)
