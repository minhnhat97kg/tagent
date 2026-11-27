import { expect, test } from 'vitest'
import { DONE_ID, START_ID, fitView, jobsToGraph, layoutGraph, type Job } from './graphLayout'

const job = (ID: string, over: Partial<Job> = {}): Job => ({ ID, Status: 'dev', Agent: 'dev', Model: '', Prompt: 'do it', Run: '', Outputs: [], OnFailure: '', MaxLoops: 0, ...over })
const at = (g: ReturnType<typeof layoutGraph>, id: string) => g.nodes.find(n => n.id === id)!
const incoming = (g: ReturnType<typeof layoutGraph>, id: string) => g.edges.filter(e => e.to === id)

test('layout fans out branches', () => {
  const { nodes, edges } = jobsToGraph([job('plan'), job('ui', { Needs: ['plan'] }), job('api', { Needs: ['plan'] })])
  const g = layoutGraph(nodes, edges)
  expect(at(g, 'ui').x).toBe(at(g, 'api').x)
  expect(at(g, 'ui').y).not.toBe(at(g, 'api').y)
  expect(at(g, 'ui').x).toBeGreaterThan(at(g, 'plan').x)
})

test('layout merges parallel steps', () => {
  const { nodes, edges } = jobsToGraph([job('plan'), job('ui', { Needs: ['plan'] }), job('api', { Needs: ['plan'] }), job('ship', { Needs: ['ui', 'api'] })])
  const g = layoutGraph(nodes, edges)
  expect(at(g, 'ship').x).toBeGreaterThan(Math.max(at(g, 'ui').x, at(g, 'api').x))
  expect(incoming(g, 'ship').map(e => e.from).sort()).toEqual(['api', 'ui'])
})

test('layout turns on-failure into loop node', () => {
  const { nodes, edges } = jobsToGraph([job('build', { Run: 'make' }), job('test', { Run: 'go test', Needs: ['build'], OnFailure: 'build', MaxLoops: 3 })])
  const g = layoutGraph(nodes, edges)
  const loop = at(g, 'loop:test')
  expect(loop.kind).toBe('loop')
  expect(loop.title).toBe('back to build')
  expect(loop.target).toBe('build')
  expect(g.edges.find(e => e.to === 'loop:test')?.label).toBe('fail · max 3')
  expect(g.edges.some(e => e.to === 'build' && e.from === 'test')).toBe(false)
  expect(g.edges.every(e => at(g, e.from).x < at(g, e.to).x)).toBe(true)
})

test('layout ignores unknown needs', () => {
  const { nodes, edges } = jobsToGraph([job('a', { Needs: ['ghost'] }), job('b', { Needs: ['a'] })])
  const g = layoutGraph(nodes, edges)
  expect(incoming(g, 'a').map(e => e.from)).toEqual([START_ID])
  expect(g.nodes.every(n => Number.isFinite(n.x) && Number.isFinite(n.y))).toBe(true)
})

test('layout survives needs cycle', () => {
  const { nodes, edges } = jobsToGraph([job('a', { Needs: ['b'] }), job('b', { Needs: ['a'] })])
  const g = layoutGraph(nodes, edges)
  expect(g.nodes.every(n => Number.isFinite(n.x) && Number.isFinite(n.y))).toBe(true)
  expect(g.edges.every(e => at(g, e.from).x < at(g, e.to).x)).toBe(true)
})

test('layout empty workflow', () => {
  const { nodes, edges } = jobsToGraph([])
  const g = layoutGraph(nodes, edges)
  expect(g.nodes.map(n => n.id).sort()).toEqual([DONE_ID, START_ID].sort())
  expect(g.edges).toHaveLength(1)
  expect(g.edges[0]).toMatchObject({ from: START_ID, to: DONE_ID })
})

test('layout chains by order without needs', () => {
  const { nodes, edges } = jobsToGraph([job('a'), job('b'), job('c')])
  const g = layoutGraph(nodes, edges)
  expect(g.edges.map(e => `${e.from}>${e.to}`)).toEqual([`${START_ID}>a`, 'a>b', 'b>c', `c>${DONE_ID}`])
  expect(at(g, 'a').x).toBeLessThan(at(g, 'b').x)
  expect(at(g, 'b').x).toBeLessThan(at(g, 'c').x)
})

test('layout is deterministic and never overlaps nodes', () => {
  const jobs = [job('plan'), job('ui', { Needs: ['plan'] }), job('api', { Needs: ['plan'] }), job('db', { Needs: ['plan'] }), job('ship', { Needs: ['ui', 'api', 'db'] })]
  const a = jobsToGraph(jobs)
  const first = layoutGraph(a.nodes, a.edges)
  expect(layoutGraph(a.nodes, a.edges)).toEqual(first)
  for (const n of first.nodes) for (const m of first.nodes) {
    if (n === m) continue
    const apart = n.x + 200 <= m.x || m.x + 200 <= n.x || n.y + 60 <= m.y || m.y + 60 <= n.y
    expect(apart).toBe(true)
  }
})

test('jobs keep their kind and failure notes', () => {
  const { nodes } = jobsToGraph([job('run', { Run: 'npm test', Agent: '', Prompt: '' }), job('dev'), job('gate', { Agent: '', Prompt: '', Model: '' }), job('blocked', { Run: 'x', OnFailure: 'nowhere' })])
  const kinds = Object.fromEntries(nodes.map(n => [n.id, n.kind]))
  expect(kinds).toMatchObject({ run: 'run', dev: 'agent', gate: 'status', blocked: 'run' })
  expect(nodes.find(n => n.id === 'blocked')?.note).toBe('fail → nowhere')
  expect(nodes.some(n => n.kind === 'loop')).toBe(false)
})

test('duplicate job ids keep one node', () => {
  const { nodes } = jobsToGraph([job('a'), job('a', { Status: 'review' })])
  expect(nodes.filter(n => n.id === 'a')).toEqual([expect.objectContaining({ status: 'review' })])
})

test('fitView shows content inside the box and never zooms past 1', () => {
  expect(fitView({ w: 400, h: 200 }, { w: 200, h: 200 })).toEqual({ k: 0.5, x: 0, y: 50 })
  expect(fitView({ w: 100, h: 100 }, { w: 400, h: 400 }).k).toBe(1)
  expect(fitView({ w: 0, h: 0 }, { w: 400, h: 400 }).k).toBe(1)
})
