export function uploadError(files: { size: number }[]) {
  return files.length > 4 || files.some(f => f.size > 10 * 1024 * 1024) ? "Maximum four files, 10 MiB each" : ""
}
export function previewText(name: string, text: string, truncated: boolean) {
  if (!truncated && name.toLowerCase().endsWith(".json")) {
    try { return JSON.stringify(JSON.parse(text), null, 2) } catch { /* invalid JSON remains readable */ }
  }
  return text
}

export const attachmentURL = (dir: string, id: string) => `/api/tasks/${encodeURIComponent(id)}/attachments?dir=${encodeURIComponent(dir)}`
