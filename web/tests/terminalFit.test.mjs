import { test } from 'node:test'
import assert from 'node:assert/strict'
import { terminalFontSize } from '../src/lib/terminalFit.ts'
test('phone font follows the live 768px breakpoint', () => {
 assert.equal(terminalFontSize(390),12)
 assert.equal(terminalFontSize(767),12)
 assert.equal(terminalFontSize(768),13)
 assert.equal(terminalFontSize(1200),13)
})
