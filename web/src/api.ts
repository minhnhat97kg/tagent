import { toast } from "sonner"

export async function api<T = unknown>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(path, init)
  const text = await r.text()
  if (!r.ok) {
    throw new Error(r.status === 401 ? "unauthorized: reload the page and enter the access token" : text || r.statusText)
  }
  const isJSON = (r.headers.get("content-type") ?? "").includes("json")
  return (isJSON && text ? JSON.parse(text) : text) as T
}

export const send = <T = unknown>(method: string, path: string, body: unknown) =>
  api<T>(path, { method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) })

export const post = <T = unknown>(path: string, body: unknown) => send<T>("POST", path, body)

// act runs a write and reports a failure as a toast.
export const act = (p: Promise<unknown>) => p.catch((e: Error) => toast.error(e.message))
