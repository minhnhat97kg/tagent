import { test } from 'node:test'
import assert from 'node:assert/strict'
import { bindVisualViewport, bindTerminalTap } from '../src/lib/touchViewport.ts'

test('simulated iPad viewport shrinks, pans, restores, and never scrolls the page', () => {
 const viewport = Object.assign(new EventTarget(), { height: 900, offsetTop: 0 })
 const win = Object.assign(new EventTarget(), { visualViewport: viewport, innerHeight: 900, scrollX: 0, scrollY: 0,
  scrollTo(x, y) { this.scrollX = x; this.scrollY = y } })
 const style = new Map()
 const root = { style: { setProperty: (k, v) => style.set(k, v), removeProperty: k => style.delete(k) } }
 const cleanup = bindVisualViewport(root, win)
 assert.equal(style.get('--app-height'), '900px')
 viewport.height = 450; viewport.offsetTop = 70
 win.scrollY = 70
 viewport.dispatchEvent(new Event('resize'))
 assert.equal(style.get('--app-height'), '450px')
 assert.equal(style.get('--app-top'), '70px')
 assert.equal(win.scrollY, 0)
 // Shell header is at visible top; its bottom (and KeyBar) is at keyboard top.
 assert.equal(parseFloat(style.get('--app-top')) + parseFloat(style.get('--app-height')), 520)
 viewport.offsetTop = 100; viewport.dispatchEvent(new Event('scroll'))
 assert.equal(style.get('--app-top'), '100px')
 win.scrollY = 20; win.dispatchEvent(new Event('scroll')); assert.equal(win.scrollY, 0)
 viewport.height = 900; viewport.offsetTop = 0; viewport.dispatchEvent(new Event('resize'))
 assert.equal(style.get('--app-height'), '900px')
 cleanup()
 viewport.dispatchEvent(new Event('resize'))
 assert.equal(style.size, 0)
})

test('fallback tracks window height without visualViewport', () => {
 const win = Object.assign(new EventTarget(), { innerHeight: 700, scrollX: 0, scrollY: 0 })
 const style = new Map()
 const cleanup = bindVisualViewport({ style: { setProperty: (k,v) => style.set(k,v), removeProperty: k => style.delete(k) } }, win)
 win.innerHeight = 400; win.dispatchEvent(new Event('resize'))
 assert.equal(style.get('--app-height'), '400px')
 assert.equal(style.get('--app-top'), '0px')
 cleanup()
})

test('only short stationary taps focus IME; drags, long presses and cancellation do not', () => {
 const el = new EventTarget()
 let time = 0, taps = 0
 const cleanup = bindTerminalTap(el, () => taps++, () => time)
 const pointer = (type, x = 0, y = 0, extra = {}) => el.dispatchEvent(Object.assign(new Event(type), { pointerId: 1, isPrimary: true, button: 0, clientX: x, clientY: y, ...extra }))
 pointer('pointerdown'); time = 399; pointer('pointerup', 7); assert.equal(taps, 1)
 pointer('pointerdown'); pointer('pointermove', 8); pointer('pointerup'); assert.equal(taps, 1)
 pointer('pointerdown'); time += 400; pointer('pointerup'); assert.equal(taps, 1)
 pointer('pointerdown'); pointer('pointerup', 0, 8); assert.equal(taps, 1)
 pointer('pointerdown'); pointer('pointercancel'); pointer('pointerup'); assert.equal(taps, 1)
 pointer('pointerdown'); pointer('pointerleave'); pointer('pointerup'); assert.equal(taps, 1)
 pointer('pointerdown'); pointer('pointerdown', 0, 0, { isPrimary: false, pointerId: 2 }); pointer('pointerup'); assert.equal(taps, 1)
 pointer('pointerdown'); time += 50; pointer('pointerup', 2, 2); assert.equal(taps, 2)
 cleanup(); pointer('pointerdown'); pointer('pointerup'); assert.equal(taps, 2)
})
