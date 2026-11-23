import { api } from "@/api"

export interface Upload { path: string; url: string; name: string; size: number; image: boolean }
export const MAX_UPLOAD = 10 * 1024 * 1024

// A file goes to the server, which stores it and returns the file path an agent on this machine can read.
export function uploadFile(file: File): Promise<Upload> {
  if (file.size > MAX_UPLOAD) return Promise.reject(new Error(`${file.name} exceeds 10 MiB`))
  const body = new FormData()
  body.append("file", file, file.name)
  return api<Upload>("/api/uploads", { method: "POST", body })
}

export const humanSize = (n: number) => n < 1024 ? `${n} B` : n < 1048576 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1048576).toFixed(1)} MB`

export interface Attached { url: string; name: string; image: boolean }
const UPLOAD_PATH = /^.*\/uploads\/([0-9a-f]{24})\/([A-Za-z0-9][A-Za-z0-9._-]*)$/
const IMAGE_EXT = /\.(?:png|jpe?g|gif|webp)$/i

// Splits a message into its words and the files it carries (lines that are an uploaded file's path).
export function splitAttachments(text: string): { text: string; files: Attached[] } {
  const files: Attached[] = []
  const rest = text.split("\n").filter(line => {
    const m = UPLOAD_PATH.exec(line.trim())
    if (m) files.push({ url: `/api/uploads/${m[1]}/${m[2]}`, name: m[2]!, image: IMAGE_EXT.test(m[2]!) })
    return !m
  })
  return { text: rest.join("\n").trim(), files }
}
