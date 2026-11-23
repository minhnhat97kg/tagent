import { useCallback, useEffect, useRef, useState } from "react"

// usePoll calls fn now and every ms while the tab is visible. Changing key refetches at once.
export function usePoll<T>(fn: () => Promise<T>, ms: number, key: unknown = "") {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<Error>()
  const fnRef = useRef(fn)
  const last = useRef("") // skip re-rendering the whole page when a poll returns the same data
  useEffect(() => {
    fnRef.current = fn
  })
  const reload = useCallback(() => {
    fnRef.current().then(
      (d) => {
        const json = JSON.stringify(d) ?? ""
        if (json !== last.current) { last.current = json; setData(d) }
        setError(undefined)
      },
      (e: Error) => setError(e),
    )
  }, [])
  useEffect(() => {
    setData(undefined)
    last.current = ""
    reload()
    const id = setInterval(() => {
      if (!document.hidden) reload()
    }, ms)
    return () => clearInterval(id)
  }, [key, ms, reload])
  return { data, error, reload }
}
