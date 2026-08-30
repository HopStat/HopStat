import {
  applyBrandPalette,
  buildBrandPalette,
  buildTerminalColors,
  normalizeHex,
  type BrandPalette,
} from '@/lib/brand-color'

export const APPEARANCE_CACHE_KEY = 'hopstat-appearance'
export const THEME_STORAGE_KEY = 'hopstat-theme'

/** Bumped whenever the set of variables written to the document changes. A cache from an
 *  older set is discarded rather than replayed, which would paint the page with variables
 *  the current stylesheet no longer understands. */
export const APPEARANCE_CACHE_VERSION = 4

export type Theme = 'light' | 'dark'

export interface AppearanceCache {
  version: number
  header_color: string
  site_name: string
  site_description: string
  theme: Theme
  brandDark: 'true' | 'false'
  vars: Record<string, string>
}

export interface AppearanceSettings {
  header_color: string
  site_name: string
  site_description?: string
}

function getSystemTheme(): Theme {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function getInitialTheme(): Theme {
  const stored = localStorage.getItem(THEME_STORAGE_KEY)
  if (stored === 'dark' || stored === 'light') return stored
  return getSystemTheme()
}

export function applyThemeClass(theme: Theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
}

function paletteToVars(palette: BrandPalette, theme: Theme): Record<string, string> {
  const { h, s } = palette.hsl
  const terminal = buildTerminalColors(h, s, theme)

  const vars: Record<string, string> = {
    '--brand': palette.brand,
    '--brand-h': String(palette.hsl.h),
    '--brand-s': `${palette.hsl.s}%`,
    '--brand-l': `${palette.hsl.l}%`,
    '--brand-foreground': palette.foreground,
    '--brand-accent': palette.accent,
    '--brand-accent-ui': palette.accentUi,
    '--brand-accent-line': palette.accentLine,
    '--brand-muted': palette.muted,
    '--brand-terminal-bg': terminal.bg,
    '--brand-terminal-border': terminal.border,
    '--brand-terminal-fg': terminal.fg,
    '--brand-terminal-muted': terminal.muted,
    '--brand-terminal-asn': terminal.asn,
  }

  if (theme === 'dark' && palette.darkSurfaces) {
    const surfaces = palette.darkSurfaces
    vars['--color-background'] = surfaces.background
    vars['--color-foreground'] = surfaces.foreground
    vars['--color-muted'] = surfaces.muted
    vars['--color-muted-foreground'] = surfaces.mutedForeground
    vars['--color-border'] = surfaces.border
    vars['--color-card'] = surfaces.card
    vars['--color-card-foreground'] = surfaces.cardForeground
    vars['--color-popover'] = surfaces.popover
    vars['--color-popover-foreground'] = surfaces.popoverForeground
    vars['--color-accent'] = surfaces.accent
    vars['--color-accent-foreground'] = surfaces.accentForeground
    vars['--color-input'] = surfaces.input
    vars['--color-surface-elevated'] = surfaces.surfaceElevated
    vars['--color-sidebar'] = surfaces.sidebar
    vars['--color-sidebar-foreground'] = surfaces.sidebarForeground
    vars['--color-sidebar-border'] = surfaces.sidebarBorder
  }

  return vars
}

export function readAppearanceCache(): AppearanceCache | null {
  try {
    const raw = localStorage.getItem(APPEARANCE_CACHE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as AppearanceCache
    if (!parsed?.vars || typeof parsed.vars !== 'object') return null
    if (parsed.version !== APPEARANCE_CACHE_VERSION) return null
    // Dark-mode surfaces are baked into the cached variables, so a cache written for the
    // other theme would paint this one with the wrong surfaces. Happens whenever the
    // system preference flips or another tab toggles the theme.
    if (parsed.theme !== getInitialTheme()) return null
    return parsed
  } catch {
    return null
  }
}

export function applyAppearanceCache(cache: AppearanceCache) {
  const root = document.documentElement
  for (const [key, value] of Object.entries(cache.vars)) {
    root.style.setProperty(key, value)
  }
  root.dataset.brandDark = cache.brandDark
}

export function saveAppearanceCache(settings: AppearanceSettings, theme: Theme) {
  const headerColor = normalizeHex(settings.header_color) ?? '#1e293b'
  const palette = buildBrandPalette(headerColor, theme)
  applyBrandPalette(palette, theme)

  const cache: AppearanceCache = {
    version: APPEARANCE_CACHE_VERSION,
    header_color: headerColor,
    site_name: settings.site_name?.trim() || 'Looking Glass',
    site_description: settings.site_description?.trim() || '',
    theme,
    brandDark: palette.isDarkBrand ? 'true' : 'false',
    vars: paletteToVars(palette, theme),
  }

  localStorage.setItem(APPEARANCE_CACHE_KEY, JSON.stringify(cache))
  return cache
}

export function cachedSiteSettings(): Partial<AppearanceSettings> {
  const cache = readAppearanceCache()
  if (!cache) return {}
  return {
    header_color: cache.header_color,
    site_name: cache.site_name,
    site_description: cache.site_description,
  }
}
