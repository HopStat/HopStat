import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { TypewriterStatus } from './typewriter-status'

vi.mock('@/contexts/i18n-context', () => ({
  useI18n: () => ({
    locale: 'en',
    t: (key: string) => ({ 'result.running': 'Running', 'result.completed': 'Completed' })[key] ?? key,
  }),
}))

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe('TypewriterStatus', () => {
  it('shows the completed label when it is not active', () => {
    render(<TypewriterStatus active={false} />)
    expect(screen.getByText('Completed')).toBeTruthy()
  })

  it('types the running label out one character at a time', () => {
    const { container } = render(<TypewriterStatus active />)
    // Nothing typed yet, but the caret is already showing.
    expect(container.querySelector('.output-terminal__cursor')).toBeTruthy()

    act(() => { vi.advanceTimersByTime(200) })
    expect(container.textContent).toContain('R')

    act(() => { vi.advanceTimersByTime(140 * 'Running'.length) })
    expect(container.textContent).toContain('Running')
  })

  // The phase follows the prop, so a finished query must swap to the completed label
  // without needing another render pass to settle.
  it('switches to completed as soon as active goes false', () => {
    const { rerender, container } = render(<TypewriterStatus active />)
    act(() => { vi.advanceTimersByTime(500) })

    rerender(<TypewriterStatus active={false} />)
    expect(container.textContent).toBe('Completed')
    expect(container.querySelector('.output-terminal__cursor')).toBeNull()
  })

  it('restarts typing when it becomes active again', () => {
    const { rerender, container } = render(<TypewriterStatus active={false} />)
    rerender(<TypewriterStatus active />)

    act(() => { vi.advanceTimersByTime(200) })
    expect(container.textContent).toContain('R')
    expect(container.textContent).not.toContain('Completed')
  })
})
