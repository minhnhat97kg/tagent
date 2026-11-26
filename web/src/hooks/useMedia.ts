import { useEffect, useState } from "react"

// useMedia is whether a media query matches now; it follows the window live.
export function useMedia(q: string) {
  const [on, setOn] = useState(() => matchMedia(q).matches)
  useEffect(() => { const m = matchMedia(q); const f = () => setOn(m.matches); m.addEventListener("change", f); return () => m.removeEventListener("change", f) }, [q])
  return on
}
