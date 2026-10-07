import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  APPEARANCE_CACHE_KEY,
  APPEARANCE_CACHE_VERSION,
  getInitialTheme,
  readAppearanceCache,
  saveAppearanceCache,
  THEME_STORAGE_KEY,
  type Theme,
} from './appearance-cache'

function customPropertiesOnRoot(): string[] {
  const style = document.documentElement.style
  return Array.from({ length: style.length }, (_, i) => style.item(i))
    .filter(name => name.startsWith('--'))
    .sort()
}

function readRawCache() {
  return JSON.parse(localStorage.getItem(APPEARANCE_CACHE_KEY)!) as { vars: Record<string, string> }
}

beforeEach(() => {
  localStorage.clear()
  document.documentElement.removeAttribute('style')
  // Pin the theme so getInitialTheme never has to reach for matchMedia.
  localStorage.setItem(THEME_STORAGE_KEY, 'light')
})

describe('the cache and the document stay in step', () => {
  // The cache replays its stored variables straight onto the document before React runs.
  // If a variable is written by applyBrandPalette but missing from the cache, a returning
  // visitor boots without it; if it is cached but no longer written, the cache keeps
  // resurrecting a variable the stylesheet has forgotten. Either way the page is wrong for
  // one paint, and nothing else in the suite would notice.
  it.each(['light', 'dark'] as Theme[])('writes exactly the same variables in %s mode', theme => {
    localStorage.setItem(THEME_STORAGE_KEY, theme)
    saveAppearanceCache({ header_color: '#e0edd4', site_name: 'LG' }, theme)

    expect(Object.keys(readRawCache().vars).sort()).toEqual(customPropertiesOnRoot())
  })

  it('carries the contrast-solved variables, not just the raw brand', () => {
    saveAppearanceCache({ header_color: '#e0edd4', site_name: 'LG' }, 'light')
    const { vars } = readRawCache()

    expect(vars['--brand']).toBe('#e0edd4')
    expect(vars['--brand-foreground']).toBe('#152033')
    expect(vars['--brand-accent-ui']).toBe(vars['--brand-accent-line'])
  })
})

describe('a cache that no longer describes this page is discarded', () => {
  it('round-trips a cache it just wrote', () => {
    saveAppearanceCache({ header_color: '#e0edd4', site_name: 'LG' }, 'light')

    const cache = readAppearanceCache()
    expect(cache?.header_color).toBe('#e0edd4')
    expect(cache?.version).toBe(APPEARANCE_CACHE_VERSION)
  })

  it('drops a cache written before the variable set changed', () => {
    saveAppearanceCache({ header_color: '#e0edd4', site_name: 'LG' }, 'light')
    const stale = { ...JSON.parse(localStorage.getItem(APPEARANCE_CACHE_KEY)!), version: APPEARANCE_CACHE_VERSION - 1 }
    localStorage.setItem(APPEARANCE_CACHE_KEY, JSON.stringify(stale))

    expect(readAppearanceCache()).toBeNull()
  })

  it('drops a cache written for the other theme', () => {
    // Dark-mode surfaces are baked into the stored variables, so replaying a dark cache
    // under the light class paints light chrome with dark surfaces.
    localStorage.setItem(THEME_STORAGE_KEY, 'dark')
    saveAppearanceCache({ header_color: '#e0edd4', site_name: 'LG' }, 'dark')
    localStorage.setItem(THEME_STORAGE_KEY, 'light')

    expect(readAppearanceCache()).toBeNull()
  })

  it('drops a cache with no variables at all', () => {
    localStorage.setItem(APPEARANCE_CACHE_KEY, JSON.stringify({ version: APPEARANCE_CACHE_VERSION, theme: 'light' }))
    expect(readAppearanceCache()).toBeNull()
  })

  it('survives unreadable storage', () => {
    localStorage.setItem(APPEARANCE_CACHE_KEY, 'not json')
    expect(readAppearanceCache()).toBeNull()
  })
})

describe('theme resolution survives a Web Storage that throws', () => {
  // Safari Private Browsing and partitioned browser profiles raise on storage access
  // instead of returning null. getInitialTheme runs before React (appearance-boot) and
  // inside a useState initializer (theme-provider), so letting that error escape takes the
  // whole page down. The stored preference is only an optimisation — the system one is a
  // complete answer — so the read must degrade rather than throw.
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  function breakLocalStorage() {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw Object.assign(new Error('The operation is insecure.'), { name: 'SecurityError' })
    })
  }

  it('falls back to the system preference instead of throwing', () => {
    // jsdom here ships no matchMedia, and the system fallback reads it. Stand one up so
    // this case measures the storage fallback rather than a missing browser API.
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: query.includes('dark'),
      media: query,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
      dispatchEvent: () => false,
      onchange: null,
    }))
    breakLocalStorage()

    expect(() => getInitialTheme()).not.toThrow()
    expect(['light', 'dark']).toContain(getInitialTheme())
  })

  it('still honours a stored preference when storage works', () => {
    localStorage.setItem(THEME_STORAGE_KEY, 'dark')
    expect(getInitialTheme()).toBe('dark')
  })
})

describe('saving appearance survives a Web Storage that throws', () => {
  // Same failure mode as the read above, on the write side. saveAppearanceCache runs from
  // a React effect (brand-style-injector) and from appearance-boot, so letting the error
  // escape would break the brand colours for the whole session. The cache only speeds up
  // the next paint — the palette is already on the document — so the settings must still
  // come back to the caller even when nothing was persisted.
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  function breakLocalStorageWrites() {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw Object.assign(new Error('The operation is insecure.'), { name: 'SecurityError' })
    })
  }

  it('applies the settings without persisting them', () => {
    breakLocalStorageWrites()
    let cache: ReturnType<typeof saveAppearanceCache> | undefined
    expect(() => {
      cache = saveAppearanceCache({ header_color: '#e0edd4', site_name: 'LG' }, 'light')
    }).not.toThrow()
    expect(cache?.header_color).toBe('#e0edd4')
    expect(cache?.site_name).toBe('LG')
    expect(cache?.vars['--brand']).toBeTruthy()
  })

  it('still persists the cache when storage works', () => {
    const cache = saveAppearanceCache({ header_color: '#e0edd4', site_name: 'LG' }, 'light')
    const raw = localStorage.getItem(APPEARANCE_CACHE_KEY)
    expect(raw).not.toBeNull()
    expect(JSON.parse(raw as string).version).toBe(APPEARANCE_CACHE_VERSION)
    expect(JSON.parse(raw as string).header_color).toBe(cache.header_color)
  })
})
