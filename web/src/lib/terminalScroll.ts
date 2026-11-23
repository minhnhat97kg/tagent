// iOS swipes scroll xterm's buffer, not the page or an outer terminal wrapper. After the finger lifts the
// scroll keeps gliding and slows down (momentum), like native iOS scrolling.
export function bindTerminalSwipe(el: HTMLElement, scroll: (pixels: number) => void) {
  let y: number | undefined
  let velocity = 0 // px per ms, smoothed over the last moves
  let lastTime = 0
  let glide = 0
  const stopGlide = () => { if (glide) cancelAnimationFrame(glide); glide = 0 }
  const start = (e: TouchEvent) => {
    stopGlide()
    velocity = 0
    lastTime = e.timeStamp
    y = e.touches.length === 1 ? e.touches[0].clientY : undefined
  }
  const move = (e: TouchEvent) => {
    if (y === undefined || e.touches.length !== 1) { y = undefined; return }
    const next = e.touches[0].clientY
    const dt = Math.max(1, e.timeStamp - lastTime)
    velocity = 0.8 * ((y - next) / dt) + 0.2 * velocity
    lastTime = e.timeStamp
    scroll(y - next)
    y = next
    e.preventDefault()
  }
  const end = () => {
    y = undefined
    if (typeof requestAnimationFrame !== 'function' || Math.abs(velocity) < 0.05) return
    let prev = performance.now()
    const step = (now: number) => {
      const dt = Math.min(32, now - prev)
      prev = now
      scroll(velocity * dt)
      velocity *= Math.pow(0.95, dt / 16)
      glide = Math.abs(velocity) > 0.02 ? requestAnimationFrame(step) : 0
    }
    glide = requestAnimationFrame(step)
  }
  el.addEventListener('touchstart', start, { passive: true })
  el.addEventListener('touchmove', move, { passive: false })
  el.addEventListener('touchend', end)
  el.addEventListener('touchcancel', end)
  return () => {
    stopGlide()
    el.removeEventListener('touchstart', start)
    el.removeEventListener('touchmove', move)
    el.removeEventListener('touchend', end)
    el.removeEventListener('touchcancel', end)
  }
}

export function snapshotBytes(screen: string, remoteRows: number, localRows: number, cx: number, cy: number, cursor: boolean, alternate: boolean) {
  const row = Math.max(1, Math.min(localRows, cy + 1 + localRows - remoteRows))
  return (alternate ? '\x1b[?1049h' : '') + '\x1b[H' + screen.split('\n').map(l => l + '\x1b[0m\x1b[K').join('\r\n') +
    `\x1b[${row};${cx + 1}H` + (cursor ? '\x1b[?25h' : '\x1b[?25l')
}

// sgrWheel encodes wheel steps as SGR mouse events (button 64 up, 65 down) at a cell, for apps that
// read the mouse themselves in the alternate screen (Claude Code, pi): that is how they scroll their history.
export function sgrWheel(up: boolean, steps: number, col: number, row: number) {
  return `\x1b[<${up ? 64 : 65};${Math.max(1, col)};${Math.max(1, row)}M`.repeat(Math.max(0, steps))
}
