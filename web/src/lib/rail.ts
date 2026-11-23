// The left rail's collapsed state, remembered per device. Storage can be
// blocked (private window), so every access is guarded.

const KEY = "tagent.rail"

export function getRailCollapsed(): boolean {
  try {
    return localStorage.getItem(KEY) === "collapsed"
  } catch {
    return false
  }
}

export function setRailCollapsed(collapsed: boolean) {
  try {
    localStorage.setItem(KEY, collapsed ? "collapsed" : "open")
  } catch {
    // not remembered
  }
}
