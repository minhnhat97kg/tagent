// Glyphs mirror the TUI dashboard. The class carries the color (from index.css tokens).
export const STATUS_GLYPH: Record<string, { glyph: string; cls: string }> = {
  running: { glyph: "●", cls: "text-primary animate-pulse" },
  background: { glyph: "◌", cls: "text-primary/60" },
  waiting: { glyph: "!", cls: "font-bold text-amber-500" },
  error: { glyph: "✖", cls: "text-red-500" },
  done: { glyph: "✔", cls: "text-green-600" },
  idle: { glyph: "○", cls: "text-muted-foreground" },
}

export const isActive = (status: string) => status === "running" || status === "background"

// shortPrompt is the first line of a prompt, cut for a one-line row.
export const shortPrompt = (s: string) => (s.trim().split("\n")[0] ?? "").slice(0, 120)

export const baseName = (p: string) => p.split("/").filter(Boolean).pop() ?? p

// ago formats a unix time in seconds as "42s", "3m" or "2h" ago.
export function ago(epochSec: number) {
  const s = Math.max(0, Date.now() / 1000 - epochSec)
  return s < 60 ? `${Math.floor(s)}s` : s < 3600 ? `${Math.floor(s / 60)}m` : `${Math.floor(s / 3600)}h`
}

export const agentLabel = (a: { name: string; agent: string }) => a.name || a.agent
