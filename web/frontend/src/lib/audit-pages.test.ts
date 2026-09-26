import { describe, expect, it } from 'vitest'
import { auditPageWindow } from './audit-pages'

describe('auditPageWindow', () => {
  it('lists every page when there are at most ten', () => {
    expect(auditPageWindow(1, 0)).toEqual([])
    expect(auditPageWindow(2, 10)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10])
  })

  it('keeps pages after ten reachable', () => {
    expect(auditPageWindow(1, 25)).toEqual([1, 2, 3, 'gap', 25])
    expect(auditPageWindow(10, 25)).toEqual([1, 'gap', 8, 9, 10, 11, 12, 'gap', 25])
    expect(auditPageWindow(25, 25)).toEqual([1, 'gap', 23, 24, 25])
  })
})
