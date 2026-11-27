import { afterEach, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { FlowCanvas } from './FlowCanvas'
import { jobsToGraph, layoutGraph, type Job } from '@/lib/graphLayout'

const job = (ID: string, over: Partial<Job> = {}): Job => ({ ID, Status: 'dev', Agent: 'dev', Model: '', Prompt: 'do it', Run: '', Outputs: [], OnFailure: '', MaxLoops: 0, ...over })
const { nodes, edges } = jobsToGraph([job('build', { Run: 'make' }), job('test', { Run: 'go test', OnFailure: 'build' })])
const graph = layoutGraph(nodes, edges)

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

test('FlowCanvas selects a node', () => {
  const onSelect = vi.fn()
  render(<FlowCanvas graph={graph} onSelect={onSelect} />)
  fireEvent.click(screen.getByRole('button', { name: /test/ }))
  expect(onSelect).toHaveBeenCalledWith('test')
})

test('FlowCanvas loop node selects its target job', () => {
  const onSelect = vi.fn()
  render(<FlowCanvas graph={graph} onSelect={onSelect} />)
  fireEvent.click(screen.getByRole('button', { name: /back to build/ }))
  expect(onSelect).toHaveBeenCalledWith('build')
})

test('FlowCanvas marks the active node', () => {
  const { rerender } = render(<FlowCanvas graph={graph} active="build" onSelect={() => {}} />)
  expect(screen.getByRole('button', { name: /^build/ }).getAttribute('aria-pressed')).toBe('true')
  expect(screen.getByRole('button', { name: /^test/ }).getAttribute('aria-pressed')).toBe('false')
  rerender(<FlowCanvas graph={graph} active="test" onSelect={() => {}} />)
  expect(screen.getByRole('button', { name: /^test/ }).getAttribute('aria-pressed')).toBe('true')
})

test('FlowCanvas zoom controls change the scale and stay within 0.4 to 2', () => {
  const { container } = render(<FlowCanvas graph={graph} onSelect={() => {}} />)
  const content = () => container.querySelector('.origin-top-left') as HTMLElement
  fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }))
  expect(content().style.transform).toContain('scale(1.25)')
  for (let i = 0; i < 20; i++) fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }))
  expect(content().style.transform).toContain('scale(2)')
  for (let i = 0; i < 40; i++) fireEvent.click(screen.getByRole('button', { name: 'Zoom out' }))
  expect(content().style.transform).toContain('scale(0.4)')
  fireEvent.click(screen.getByRole('button', { name: 'Fit to view' }))
  expect(content().style.transform).toContain('scale(')
})

test('FlowCanvas renders without ResizeObserver', () => {
  vi.stubGlobal('ResizeObserver', undefined)
  render(<FlowCanvas graph={graph} onSelect={() => {}} />)
  expect(screen.getByRole('region', { name: 'Flow canvas' })).toBeTruthy()
})
