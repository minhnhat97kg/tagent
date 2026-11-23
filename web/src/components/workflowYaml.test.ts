import { describe, expect, it } from 'vitest'
import { jobRanges, lineShape } from './workflowYaml'

describe('lineShape', () => {
  it('puts wrapped rows under the value, past a list dash', () => {
    expect(lineShape('    - prompt: long text')).toEqual({ v: 14, s: 4 })
    expect(lineShape('plain text')).toBeNull()
  })
})

describe('jobRanges', () => {
  it('spans each job from its key to the line before the next job', () => {
    const doc = ['name: x', 'jobs:', '  build:', '    agent: a', '', '  test:', '    run: go test', '    needs: build', 'other: 1'].join('\n')
    expect(jobRanges(doc)).toEqual({ build: { from: 3, to: 4 }, test: { from: 6, to: 8 } })
  })
  it('returns nothing without a jobs key and survives a half-typed job', () => {
    expect(jobRanges('name: x')).toEqual({})
    expect(jobRanges('jobs:\n  a:\n    run: x\n  b:')).toEqual({ a: { from: 2, to: 3 }, b: { from: 4, to: 4 } })
  })
})
