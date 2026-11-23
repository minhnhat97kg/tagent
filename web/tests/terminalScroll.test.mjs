import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { snapshotBytes, bindTerminalSwipe } from '../src/lib/terminalScroll.ts'
globalThis.self = globalThis
const { Terminal } = createRequire(import.meta.url)('@xterm/xterm')
const write = (term, data) => new Promise(resolve => term.write(data, resolve))

test('snapshot seeds history and restores cursor; live output preserves a scrolled-up view', async () => {
 const term = new Terminal({ cols: 80, rows: 24, scrollback: 5000, scrollOnUserInput: false, allowProposedApi: true })
 try {
  const screen = Array.from({length: 224}, (_,i) => `line ${i}`).join('\n')
  await write(term, snapshotBytes(screen,24,24,7,23,true,false))
  assert.equal(term.buffer.active.baseY,200)
  assert.equal(term.buffer.active.getLine(0).translateToString(true),'line 0')
  assert.equal(term.buffer.active.cursorX,7)
  assert.equal(term.buffer.active.cursorY,23)
  term.scrollLines(-50)
  const before = term.buffer.active.viewportY
  await write(term,'\r\nnew output')
  assert.equal(term.buffer.active.viewportY,before)
  term.scrollToBottom()
  await write(term,'\r\nfollowing output')
  assert.equal(term.buffer.active.viewportY,term.buffer.active.baseY)
 } finally { term.dispose() }
})

test('attached-pane fallback uses local rows without an outer scroll container', async () => {
 const term = new Terminal({cols:80,rows:10,scrollback:5000,allowProposedApi:true})
 try {
  await write(term,snapshotBytes(Array.from({length:124},(_,i)=>`row ${i}`).join('\n'),24,10,2,23,true,false))
  assert.equal(term.buffer.active.baseY,114)
  assert.equal(term.buffer.active.cursorY,9)
  term.scrollToTop()
  assert.equal(term.buffer.active.viewportY,0)
 } finally { term.dispose() }
})

test('alternate snapshot selects alternate buffer, whose wheel must be blocked', async () => {
 const term = new Terminal({cols:80,rows:10,allowProposedApi:true})
 try {
  await write(term,snapshotBytes('full-screen app',10,10,3,0,true,true))
  assert.equal(term.buffer.active.type,'alternate')
  assert.equal(term.buffer.active.cursorX,3)
 } finally { term.dispose() }
})

test('touch swipes scroll terminal pixels and prevent page scrolling; cleanup stops handling', () => {
 const el = new EventTarget(); const deltas = []
 const cleanup = bindTerminalSwipe(el, delta => deltas.push(delta))
 const touch = (type,y) => {
  const e = Object.assign(new Event(type,{cancelable:true}),{touches:y === undefined ? [] : [{clientY:y}]})
  el.dispatchEvent(e); return e
 }
 touch('touchstart',200)
 assert.equal(touch('touchmove',150).defaultPrevented,true)
 touch('touchmove',175)
 assert.deepEqual(deltas,[50,-25])
 touch('touchend'); touch('touchmove',100)
 assert.deepEqual(deltas,[50,-25])
 cleanup(); touch('touchstart',200);touch('touchmove',0)
 assert.deepEqual(deltas,[50,-25])
})

test('sgrWheel encodes wheel steps for mouse-reading apps', async () => {
  const { sgrWheel } = await import('../src/lib/terminalScroll.ts')
  assert.equal(sgrWheel(true, 2, 10, 5), '\x1b[<64;10;5M\x1b[<64;10;5M')
  assert.equal(sgrWheel(false, 1, 0, 0), '\x1b[<65;1;1M')
  assert.equal(sgrWheel(true, 0, 3, 3), '')
})
