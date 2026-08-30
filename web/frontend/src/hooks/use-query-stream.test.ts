import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useQueryStream } from './use-sse'

/** Minimal stand-in for EventSource: jsdom has none, and the hook is built around one. */
class StubEventSource {
  static instances: StubEventSource[] = []
  url: string
  closed = false
  onerror: (() => void) | null = null
  private listeners = new Map<string, Array<(e: MessageEvent) => void>>()

  constructor(url: string) {
    this.url = url
    StubEventSource.instances.push(this)
  }

  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    const list = this.listeners.get(type) ?? []
    list.push(fn)
    this.listeners.set(type, list)
  }

  close() {
    this.closed = true
  }

  /** Deliver a server event to the hook. */
  emit(type: string, data: unknown) {
    for (const fn of this.listeners.get(type) ?? []) {
      fn({ data: JSON.stringify(data) } as MessageEvent)
    }
  }
}

const latest = () => StubEventSource.instances[StubEventSource.instances.length - 1]

beforeEach(() => {
  StubEventSource.instances = []
  vi.stubGlobal('EventSource', StubEventSource)
})
afterEach(() => vi.unstubAllGlobals())

describe('useQueryStream', () => {
  it('stays idle without a query id', () => {
    const { result } = renderHook(() => useQueryStream(null))
    expect(result.current.result).toBeNull()
    expect(result.current.lines).toEqual([])
    expect(StubEventSource.instances).toHaveLength(0)
  })

  it('opens the stream for a query and collects its output', () => {
    const { result } = renderHook(() => useQueryStream('abc'))
    expect(latest().url).toBe('/api/v1/query/abc/stream')

    act(() => { latest().emit('output', { line: 'first' }) })
    act(() => { latest().emit('output', { line: 'second' }) })
    expect(result.current.lines).toEqual(['first', 'second'])

    act(() => { latest().emit('output_done', {}) })
    expect(result.current.outputComplete).toBe(true)

    act(() => { latest().emit('result', { id: 'abc', status: 'done', raw: '', lines: [] }) })
    expect(result.current.result?.status).toBe('done')
  })

  // The reset on a new query is the behaviour most at risk of regressing: without it the
  // previous query's output would still be on screen while the new one runs.
  it('drops the previous query’s state when the id changes', () => {
    const { result, rerender } = renderHook(({ id }) => useQueryStream(id), {
      initialProps: { id: 'first' as string | null },
    })
    act(() => { latest().emit('output', { line: 'stale' }) })
    act(() => { latest().emit('output_done', {}) })
    expect(result.current.lines).toEqual(['stale'])

    rerender({ id: 'second' })
    expect(result.current.lines).toEqual([])
    expect(result.current.result).toBeNull()
    expect(result.current.outputComplete).toBe(false)
    expect(latest().url).toBe('/api/v1/query/second/stream')
  })

  it('clears everything when the query id goes away', () => {
    const { result, rerender } = renderHook(({ id }) => useQueryStream(id), {
      initialProps: { id: 'abc' as string | null },
    })
    act(() => { latest().emit('output', { line: 'x' }) })
    expect(result.current.lines).toEqual(['x'])

    rerender({ id: null })
    expect(result.current.lines).toEqual([])
    expect(result.current.result).toBeNull()
  })

  it('closes the stream it opened when the id changes', () => {
    const { rerender } = renderHook(({ id }) => useQueryStream(id), {
      initialProps: { id: 'first' as string | null },
    })
    const first = latest()
    rerender({ id: 'second' })
    expect(first.closed).toBe(true)
  })
})
