import { test } from 'node:test'
import assert from 'node:assert/strict'
import { uploadError, previewText } from '../src/lib/attachments.ts'

test('upload bounds allow exactly four files and ten MiB each', () => {
  assert.equal(uploadError(Array(4).fill({size: 10 * 1024 * 1024})), '')
  assert.ok(uploadError(Array(5).fill({size: 1})))
  assert.ok(uploadError([{size: 10 * 1024 * 1024 + 1}]))
})
test('JSON previews pretty print only complete valid JSON', () => {
  assert.equal(previewText('a.json', '{"ok":true}', false), '{\n  "ok": true\n}')
  assert.equal(previewText('a.json', '{"ok":true}', true), '{"ok":true}')
  assert.equal(previewText('a.json', '{broken', false), '{broken')
  assert.equal(previewText('a.md', '<script>alert(1)</script>', false), '<script>alert(1)</script>')
})
