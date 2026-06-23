import { describe, expect, it } from 'vitest'
import { compareVersions, shouldShowPostUpdateWhatsNew } from './version-seen'

describe('version-seen', () => {
  it('compares semver parts', () => {
    expect(compareVersions('v2.1.63', 'v2.1.62')).toBeGreaterThan(0)
    expect(compareVersions('v2.1.62', 'v2.1.63')).toBeLessThan(0)
    expect(compareVersions('v2.1.62', 'v2.1.62')).toBe(0)
  })

  it('shows post-update when current is newer than last seen', () => {
    localStorage.setItem('hopstat_admin_last_seen_version', 'v2.1.62')
    expect(shouldShowPostUpdateWhatsNew('v2.1.63')).toBe(true)
    expect(shouldShowPostUpdateWhatsNew('v2.1.62')).toBe(false)
    localStorage.removeItem('hopstat_admin_last_seen_version')
  })

  it('skips post-update when no baseline version exists', () => {
    localStorage.removeItem('hopstat_admin_last_seen_version')
    expect(shouldShowPostUpdateWhatsNew('v2.1.63')).toBe(false)
  })
})
