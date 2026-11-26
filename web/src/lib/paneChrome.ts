import { createContext, type PointerEvent, type ReactNode } from "react"

// What a pane's header gets from where the pane sits: on the desktop home a grip, a drag handle and a
// maximize link; on a maximized page a minimize link. Without it (a phone, a nested panel), nothing.
export type PaneChrome = { id: string; maximized?: boolean; grip?: ReactNode; onPointerDown?: (e: PointerEvent<HTMLElement>) => void }

export const PaneChromeContext = createContext<PaneChrome | null>(null)
