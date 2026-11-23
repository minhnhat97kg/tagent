import { useCallback, useEffect, useRef, useState } from "react"

// usePoll calls fn now and every ms while the tab is visible. Changing key refetches at once.
export function usePoll<T>(fn: () => Promise<T>, ms: number, key: unknown = "") {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<Error>()
  const fnRef = useRef(fn)
  useEffect(() => {
    fnRef.current = fn
  })
  const reload = useCallback(() => {
    fnRef.current().then(
      (d) => {
        setData(d)
        setError(undefined)
      },
      (e: Error) => setError(e),
    )
  }, [])
  useEffect(() => {
    setData(undefined)
    reload()
    const id = setInterval(() => {
      if (!document.hidden) reload()
    }, ms)
    return () => clearInterval(id)
  }, [key, ms, reload])
  return { data, error, reload }
}
