// The Usage panel rows hidden on this device. Only hidden ids are saved, so everything (also a new row) shows by default.

const KEY = "tagent.usage"

// loadHidden is this device's hidden rows; nothing saved or an unreadable value hides none.
export function loadHidden(): string[] {
  try {
    const s = JSON.parse(localStorage.getItem(KEY) ?? "null")
    return s?.v === 1 && Array.isArray(s.hidden) ? s.hidden.filter((x: unknown) => typeof x === "string") : []
  } catch {
    return []
  }
}

export function saveHidden(hidden: string[]) {
  try {
    if (hidden.length) localStorage.setItem(KEY, JSON.stringify({ v: 1, hidden }))
    else localStorage.removeItem(KEY)
  } catch { /* not remembered in private mode */ }
}
