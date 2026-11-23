import { describe, expect, it } from 'vitest'
import { entryLabel, findAttachment, formatDuration, resultChip } from './run'

describe('entryLabel', () => {
  it('keeps a first entry plain', () => {
    expect(entryLabel({ step: 'build', loop: 1, result: 'current' })).toBe('build')
  })
  it('names the loop and the step that sent it back', () => {
    expect(entryLabel({ step: 'build', loop: 2, result: 'current' }, { step: 'review', loop: 1, result: 'changes' })).toBe('build #2 after review: changes')
  })
  it('explains a resume and a retry', () => {
    expect(entryLabel({ step: 'build', loop: 2, result: 'current' }, { step: 'build', loop: 1, result: 'blocked' })).toBe('build #2 after blocked')
    expect(entryLabel({ step: 'build', loop: 2, result: 'current' }, { step: 'build', loop: 1, result: 'retry' })).toBe('build #2 after retry')
  })
})

describe('resultChip', () => {
  it('labels the results the server derives', () => {
    expect(resultChip('changes').label).toBe('changes requested')
    expect(resultChip('current').label).toBe('current')
    expect(resultChip('something-new')).toEqual({ label: 'something-new', cls: 'bg-muted text-muted-foreground' })
  })
})

describe('formatDuration', () => {
  it('reads nanoseconds as minutes and hours', () => {
    expect(formatDuration(12e9)).toBe('12s')
    expect(formatDuration(184e9)).toBe('3m 4s')
    expect(formatDuration(120e9)).toBe('2m')
    expect(formatDuration(3720e9)).toBe('1h 2m')
  })
})

describe('findAttachment', () => {
  const items = [
    { name: 'ci.log', commentId: 'c1' },
    { name: 'ci.log', commentId: 'c2' },
    { name: 'plan.md' },
  ]
  it('picks the attachment of that comment, not an earlier loop', () => {
    expect(findAttachment(items, 'ci.log', 'c2')).toBe(items[1])
    expect(findAttachment(items, 'ci.log', 'c3')).toBeUndefined()
  })
  it('matches an output file, which has no comment id', () => {
    expect(findAttachment(items, 'plan.md', 'c2')).toBe(items[2])
    expect(findAttachment(items, 'plan.md')).toBe(items[2])
  })
})
