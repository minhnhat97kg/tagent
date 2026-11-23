export type JobRange = { from: number; to: number } // 1-based, inclusive lines

// lineShape gives a wrapped line's padding: v = column of the value, s = leading indent. The wrapped
// rows start at v, so a long prompt stays under its key. Null when the line has no key or list dash.
export function lineShape(text: string): { v: number; s: number } | null {
  const m = /^(\s*)(- )?([^#]*?:\s)?/.exec(text)
  if (!m) return null
  const s = m[1].length, v = s + (m[2]?.length ?? 0) + (m[3]?.length ?? 0)
  return v > s ? { v, s } : null
}

// jobRanges maps each job under the top-level `jobs:` key to the lines it spans. Works on
// half-typed YAML too, since it only looks at indentation.
export function jobRanges(doc: string): Record<string, JobRange> {
  const lines = doc.split('\n'), out: Record<string, JobRange> = {}
  const start = lines.findIndex(l => /^jobs:/.test(l))
  if (start < 0) return out
  let indent = -1, id = '', from = 0, end = lines.length
  const close = (to: number) => {
    if (!id) return
    while (to > from && !lines[to - 1].trim()) to--
    out[id] = { from, to }
  }
  for (let i = start + 1; i < lines.length; i++) {
    const line = lines[i]
    if (!line.trim() || /^\s*#/.test(line)) continue
    const ind = line.length - line.trimStart().length
    if (ind === 0) { end = i; break }
    if (indent < 0) indent = ind
    const key = ind === indent ? /^\s*([^\s#'"][^:]*):(\s|$)/.exec(line) : null
    if (key) { close(i); id = key[1].trim(); from = i + 1 }
  }
  close(end)
  return out
}
