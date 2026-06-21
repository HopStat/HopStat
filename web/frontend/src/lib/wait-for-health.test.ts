import { describe, expect, it, vi } from 'vitest'
import { normalizeVersion, waitForHealth } from './wait-for-health'

describe('normalizeVersion', () => {
  it('strips leading v', () => {
    expect(normalizeVersion('v2.1.22')).toBe('2.1.22')
  })
})

describe('waitForHealth', () => {
  it('waits until health is ok and version matches', async () => {
    vi.useFakeTimers()
    const fetchHealth = vi.fn<() => Promise<{ status?: string; version?: string } | null>>()
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ status: 'ok', version: 'v2.1.21' })
      .mockResolvedValueOnce({ status: 'ok', version: 'v2.1.22' })

    const promise = waitForHealth({
      expectedVersion: 'v2.1.22',
      initialDelayMs: 100,
      intervalMs: 100,
      maxAttempts: 5,
      fetchHealth,
    })

    await vi.advanceTimersByTimeAsync(100)
    await vi.advanceTimersByTimeAsync(100)
    await vi.advanceTimersByTimeAsync(100)

    await expect(promise).resolves.toEqual({ status: 'ok', version: 'v2.1.22' })
    expect(fetchHealth).toHaveBeenCalledTimes(3)
    vi.useRealTimers()
  })

  it('accepts first ok health when no expected version', async () => {
    vi.useFakeTimers()
    const fetchHealth = vi.fn().mockResolvedValue({ status: 'ok', version: 'v2.1.22' })

    const promise = waitForHealth({
      initialDelayMs: 0,
      intervalMs: 50,
      fetchHealth,
    })

    await vi.runAllTimersAsync()
    await expect(promise).resolves.toEqual({ status: 'ok', version: 'v2.1.22' })
    vi.useRealTimers()
  })

  it('throws when service never becomes ready', async () => {
    vi.useFakeTimers()
    const fetchHealth = vi.fn().mockResolvedValue(null)

    const promise = waitForHealth({
      initialDelayMs: 0,
      intervalMs: 10,
      maxAttempts: 2,
      fetchHealth,
    })

    const expectation = expect(promise).rejects.toThrow('service not ready')
    await vi.runAllTimersAsync()
    await expectation
    vi.useRealTimers()
  })
})
