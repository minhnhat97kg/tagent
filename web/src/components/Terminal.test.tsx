import { afterEach, expect, test, vi } from "vitest"
import { act, cleanup, render } from "@testing-library/react"
import { createElement } from "react"

// Stubs that must exist before Terminal.tsx loads: it reads matchMedia at module scope (touch mode).
vi.hoisted(() => {
  window.matchMedia = (() => ({ matches: true, addEventListener() {}, removeEventListener() {} })) as unknown as typeof window.matchMedia
  globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} } as unknown as typeof ResizeObserver
  Object.defineProperty(document, "fonts", { configurable: true, value: { ready: Promise.resolve(), addEventListener() {}, removeEventListener() {} } })
})

vi.mock("@xterm/xterm", () => ({
  Terminal: class {
    options: Record<string, unknown> = {}
    buffer = { active: { type: "normal" } }
    rows = 24
    cols = 80
    textarea = undefined
    open() {}
    loadAddon() {}
    attachCustomKeyEventHandler() {}
    attachCustomWheelEventHandler() {}
    onData() { return { dispose() {} } }
    write() {}
    reset() {}
    resize() {}
    scrollLines() {}
    dispose() {}
  },
}))
vi.mock("@xterm/addon-webgl", () => ({ WebglAddon: class { onContextLoss() {} dispose() {} } }))
vi.mock("@/api", () => ({ post: vi.fn(async () => ({ queued: false })) }))
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }))
vi.mock("@/hooks/useRoute", () => ({ enc: (s: string) => encodeURIComponent(s) }))

// Every WebSocket records what the pane receives; the socket is always open.
const sockets: FakeSocket[] = []
class FakeSocket {
  static OPEN = 1
  readyState = 1
  binaryType = ""
  onopen?: () => void
  onmessage?: (e: { data: unknown }) => void
  onclose?: () => void
  onerror?: () => void
  received: string[] = []
  constructor() { sockets.push(this) }
  send(data: Uint8Array) { this.received.push(new TextDecoder().decode(data)) }
  close() { this.readyState = 3 }
}
vi.stubGlobal("WebSocket", FakeSocket)

const flush = () => new Promise(resolve => setTimeout(resolve, 0))

afterEach(() => { cleanup(); sockets.length = 0 })

// The pane's view of the keys: DEL (\x7f) removes the previous character.
function paneText(keys: string[]) {
  const out: string[] = []
  for (const k of keys.join("")) { if (k === "\x7f") out.pop(); else out.push(k) }
  return out.join("")
}

// Types the way a predictive Android keyboard does: the textarea holds the whole composing word,
// and an input event follows every composition update.
function imeInput(box: HTMLTextAreaElement, value: string, isComposing = true) {
  box.value = value
  box.dispatchEvent(new InputEvent("input", { bubbles: true, isComposing }))
}

test("a predictive IME composition sends each character once, even across a space", async () => {
  const { Terminal } = await import("./Terminal")
  render(createElement(Terminal, { pane: "%composition" }))
  await act(flush)
  const box = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="Type into terminal"]')!
  expect(box).toBeTruthy()

  box.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true, data: "" }))
  for (const word of ["h", "he", "hel", "hell", "hello"]) {
    imeInput(box, word)
    box.dispatchEvent(new CompositionEvent("compositionupdate", { bubbles: true, data: word }))
  }
  // The space commits "hello" but the keyboard keeps composing; this event reports isComposing=false.
  imeInput(box, "hello ", false)
  // The keyboard rewrites its still-open composition into the box, which now holds "hello w".
  imeInput(box, "hello w")
  box.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "hello w" }))
  await act(flush)

  expect(paneText(sockets[0].received)).toBe("hello w")
})

test("Enter during a predictive composition does not reset the box and re-send the word", async () => {
  const { Terminal } = await import("./Terminal")
  render(createElement(Terminal, { pane: "%composition-enter" }))
  await act(flush)
  const box = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="Type into terminal"]')!

  box.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true, data: "" }))
  imeInput(box, "hello", false)
  // Samsung reports Enter keydown with isComposing=false while the word is still composing.
  box.dispatchEvent(new KeyboardEvent("keydown", { bubbles: true, key: "Enter", isComposing: false }))
  imeInput(box, "hello w")
  box.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "hello w" }))
  await act(flush)

  expect(paneText(sockets[0].received)).toBe("hello w")
})
