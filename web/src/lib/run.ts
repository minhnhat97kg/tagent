// Helpers for the run view: row labels, result chips and durations.
export type RunRow = { step: string; loop: number; result: string }

// entryLabel names an entry and why the step ran again: "build #2 after review: changes".
export function entryLabel(e: RunRow, prev?: RunRow) {
  if (e.loop < 2) return e.step
  const label = `${e.step} #${e.loop}`
  if (!prev) return label
  if (prev.result === 'blocked') return `${label} after blocked`
  if (prev.step === e.step) return `${label} after retry`
  return `${label} after ${prev.step}: ${prev.result}`
}

const CHIPS: Record<string, { label: string; cls: string }> = {
  pass: { label: 'pass', cls: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' },
  fail: { label: 'fail', cls: 'bg-red-500/15 text-red-700 dark:text-red-400' },
  changes: { label: 'changes requested', cls: 'bg-amber-500/15 text-amber-700 dark:text-amber-400' },
  blocked: { label: 'blocked', cls: 'bg-red-500/15 text-red-700 dark:text-red-400' },
  retry: { label: 'retry', cls: 'bg-amber-500/15 text-amber-700 dark:text-amber-400' },
  moved: { label: 'moved', cls: 'bg-muted text-muted-foreground' },
  current: { label: 'current', cls: 'bg-foreground text-background' },
}

// resultChip is the colour-coded result of one entry; unknown values stay readable.
export const resultChip = (result: string) => CHIPS[result] ?? { label: result || 'unknown', cls: 'bg-muted text-muted-foreground' }

// AttachmentRef is the part of a ticket attachment that matters here.
export type AttachmentRef = { name: string; commentId?: string }

// findAttachment matches the comment's own file first, else a ticket output file
// of that name (outputs carry no comment id). Files of other comments never
// match, so each loop keeps its own attachments.
export function findAttachment<T extends AttachmentRef>(items: T[], name: string, commentId?: string): T | undefined {
  return (commentId ? items.find(a => a.name === name && a.commentId === commentId) : undefined) ?? items.find(a => a.name === name && !a.commentId)
}

// formatDuration turns the go time.Duration (ns) into "1h 02m", "3m 4s" or "12s".
export function formatDuration(ns: number) {
  const s = Math.max(0, Math.round(ns / 1e9))
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return s % 60 ? `${m}m ${s % 60}s` : `${m}m`
  return `${Math.floor(m / 60)}h ${m % 60}m`
}
