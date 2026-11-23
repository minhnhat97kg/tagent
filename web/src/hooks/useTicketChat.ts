import { useCallback, useEffect, useState } from "react"
import { api, post } from "@/api"

// TicketChat is GET/POST /api/tickets/<id>/chat: the pane bound to the ticket
// (empty until the chat starts), and the reason Chat cannot be shown when it
// cannot. The conversation itself is that pane's transcript, so the same
// endpoints as an agent's Chat view read and feed it.
export interface TicketChat {
  key: string
  pane: string
  harness: string
  status: string
  available: boolean
  reason?: string
  created?: boolean
}

// useTicketChat reports a ticket's chat pane and starts it on request. GET never
// spawns anything: the tab shows "start the chat" until the human asks for it.
export function useTicketChat(dir: string, id: string) {
  const path = `/api/tickets/${encodeURIComponent(id)}/chat?dir=${encodeURIComponent(dir)}`
  const [info, setInfo] = useState<TicketChat>()
  const [error, setError] = useState("")
  const [starting, setStarting] = useState(false)
  const reload = useCallback(async () => {
    try {
      setInfo(await api<TicketChat>(path))
      setError("")
    } catch (e) {
      setError((e as Error).message)
    }
  }, [path])
  useEffect(() => {
    setInfo(undefined)
    setError("")
    void reload()
  }, [reload])
  const start = useCallback(async (agent?: string) => {
    setStarting(true)
    try {
      setInfo(await post<TicketChat>(path, agent ? { Agent: agent } : {}))
      setError("")
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setStarting(false)
    }
  }, [path])
  return { info, error, reload, start, starting }
}
