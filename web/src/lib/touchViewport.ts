// Keep fixed chrome inside the visible viewport when iOS opens/pans its keyboard.
export function bindVisualViewport(root: HTMLElement, win: Window = window) {
  const viewport = win.visualViewport
  const sync = () => {
    root.style.setProperty('--app-height', `${viewport?.height ?? win.innerHeight}px`)
    root.style.setProperty('--app-top', `${viewport?.offsetTop ?? 0}px`)
    if (win.scrollX || win.scrollY) win.scrollTo(0, 0)
  }
  sync()
  viewport?.addEventListener('resize', sync)
  viewport?.addEventListener('scroll', sync)
  win.addEventListener('resize', sync)
  win.addEventListener('scroll', sync)
  return () => {
    viewport?.removeEventListener('resize', sync)
    viewport?.removeEventListener('scroll', sync)
    win.removeEventListener('resize', sync)
    win.removeEventListener('scroll', sync)
    root.style.removeProperty('--app-height')
    root.style.removeProperty('--app-top')
  }
}

// Do not prevent native scrolling. Reject drags even when they end back at the origin.
export function bindTerminalTap(el: HTMLElement, onTap: () => void, now = () => performance.now()) {
  let start: { id: number; x: number; y: number; time: number; moved: boolean } | undefined
  const down = (e: PointerEvent) => {
    if (!e.isPrimary || e.button !== 0) { start = undefined; return }
    start = { id: e.pointerId, x: e.clientX, y: e.clientY, time: now(), moved: false }
  }
  const move = (e: PointerEvent) => {
    if (start?.id === e.pointerId && Math.hypot(e.clientX - start.x, e.clientY - start.y) >= 8) start.moved = true
  }
  const up = (e: PointerEvent) => {
    move(e)
    const tap = start
    start = undefined
    if (tap?.id === e.pointerId && !tap.moved && now() - tap.time < 400) onTap()
  }
  const cancel = () => { start = undefined }
  el.addEventListener('pointerdown', down, { passive: true })
  el.addEventListener('pointermove', move, { passive: true })
  el.addEventListener('pointerup', up, { passive: true })
  el.addEventListener('pointercancel', cancel, { passive: true })
  el.addEventListener('pointerleave', cancel, { passive: true })
  return () => {
    el.removeEventListener('pointerdown', down)
    el.removeEventListener('pointermove', move)
    el.removeEventListener('pointerup', up)
    el.removeEventListener('pointercancel', cancel)
    el.removeEventListener('pointerleave', cancel)
  }
}
