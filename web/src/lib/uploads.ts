import { api } from "@/api"

export interface Upload { path: string; url: string; name: string }
export const MAX_IMAGE = 10 * 1024 * 1024

// An image goes to the server, which stores it and returns the file path an agent on this machine can read.
export function uploadImage(file: File): Promise<Upload> {
  if (!file.type.startsWith("image/")) return Promise.reject(new Error("Only images can be attached"))
  if (file.size > MAX_IMAGE) return Promise.reject(new Error("Image exceeds 10 MiB"))
  const body = new FormData()
  body.append("file", file, file.name)
  return api<Upload>("/api/uploads", { method: "POST", body })
}

const UPLOAD_PATH = /^.*\/uploads\/([0-9a-f]{24}\.(?:png|jpg|gif|webp))$/

// Splits a message into its words and the images it carries (lines that are an uploaded file's path).
export function splitImages(text: string): { text: string; images: string[] } {
  const images: string[] = []
  const rest = text.split("\n").filter(line => {
    const m = UPLOAD_PATH.exec(line.trim())
    if (m) images.push(`/api/uploads/${m[1]}`)
    return !m
  })
  return { text: rest.join("\n").trim(), images }
}
