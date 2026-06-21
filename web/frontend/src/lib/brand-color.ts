export const DEFAULT_BRAND = '#1e293b'

const DARK_BRAND_LIGHTNESS_THRESHOLD = 48

const DARK_SURFACE_VARS = [
  '--color-background',
  '--color-foreground',
  '--color-muted',
  '--color-muted-foreground',
  '--color-border',
  '--color-card',
  '--color-card-foreground',
  '--color-popover',
  '--color-popover-foreground',
  '--color-accent',
  '--color-accent-foreground',
  '--color-input',
  '--color-surface-elevated',
  '--color-sidebar',
  '--color-sidebar-foreground',
  '--color-sidebar-border',
] as const

export interface BrandDarkSurfaces {
  background: string
  foreground: string
  muted: string
  mutedForeground: string
  border: string
  card: string
  cardForeground: string
  popover: string
  popoverForeground: string
  accent: string
  accentForeground: string
  input: string
  surfaceElevated: string
  sidebar: string
  sidebarForeground: string
  sidebarBorder: string
}

export interface BrandPalette {
  brand: string
  header: string
  isDarkBrand: boolean
  onDark: string
  darkest: string
  accentLine: string
  formFrom: string
  formTo: string
  dark: string
  light: string
  hsl: { h: number; s: number; l: number }
  darkSurfaces?: BrandDarkSurfaces
}

export function normalizeHex(hex: string | undefined | null): string | null {
  const raw = (hex ?? '').trim()
  const long = raw.match(/^#?([a-f\d]{6})$/i)
  if (long) return `#${long[1].toLowerCase()}`
  const short = raw.match(/^#?([a-f\d]{3})$/i)
  if (!short) return null
  const [r, g, b] = short[1].split('')
  return `#${r}${r}${g}${g}${b}${b}`.toLowerCase()
}

export function hexToHSL(hex: string): { h: number; s: number; l: number } | null {
  const normalized = normalizeHex(hex)
  if (!normalized) return null
  const m = normalized.match(/^#([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i)
  if (!m) return null
  let r = parseInt(m[1], 16) / 255
  let g = parseInt(m[2], 16) / 255
  let b = parseInt(m[3], 16) / 255
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  let h = 0
  let s = 0
  const l = (max + min) / 2
  if (max !== min) {
    const d = max - min
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
    switch (max) {
      case r: h = ((g - b) / d + (g < b ? 6 : 0)) / 6; break
      case g: h = ((b - r) / d + 2) / 6; break
      case b: h = ((r - g) / d + 4) / 6; break
    }
  }
  return { h: Math.round(h * 360), s: Math.round(s * 100), l: Math.round(l * 100) }
}

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n))
}

function hsl(h: number, s: number, l: number): string {
  return `hsl(${h} ${clamp(s, 0, 100)}% ${clamp(l, 0, 100)}%)`
}

export interface BrandTerminalColors {
  bg: string
  border: string
  fg: string
  muted: string
  asn: string
}

/** Terminal output colors — dark console in dark theme, light surface in light theme. */
export function buildTerminalColors(h: number, s: number, theme: 'light' | 'dark'): BrandTerminalColors {
  const sat = clamp(Math.max(s, 20), 18, 70)
  if (theme === 'dark') {
    return {
      bg: hsl(h, sat, 8),
      border: hsl(h, sat, 16),
      fg: hsl(h, clamp(s - 20, 6, 30), 82),
      muted: hsl(h, clamp(s - 16, 8, 36), 52),
      asn: hsl(h, clamp(s + 10, 42, 78), 72),
    }
  }

  const lightSat = clamp(s - 24, 6, 28)
  const asnSat = clamp(Math.max(s, 38), 42, 72)
  return {
    bg: hsl(h, lightSat, 97),
    border: hsl(h, lightSat, 88),
    fg: hsl(h, clamp(s - 8, 14, 38), 24),
    muted: hsl(h, clamp(s - 12, 10, 32), 46),
    asn: hsl(h, asnSat, 36),
  }
}

export function isDarkBrandColor(hex: string): boolean {
  const parsed = hexToHSL(hex || DEFAULT_BRAND)
  return parsed != null && parsed.l <= DARK_BRAND_LIGHTNESS_THRESHOLD
}

/** Brand-tinted page surfaces for dark mode. */
export function buildBrandDarkSurfaces(h: number, s: number): BrandDarkSurfaces {
  const sat = clamp(Math.max(s, 22), 18, 70)
  const textSat = clamp(s - 22, 6, 34)

  return {
    background: hsl(h, sat, 7),
    foreground: hsl(h, textSat, 93),
    muted: hsl(h, sat, 11),
    mutedForeground: hsl(h, clamp(s - 12, 8, 42), 58),
    border: hsl(h, sat, 17),
    card: hsl(h, sat, 10),
    cardForeground: hsl(h, textSat, 93),
    popover: hsl(h, sat, 11),
    popoverForeground: hsl(h, textSat, 93),
    accent: hsl(h, sat, 14),
    accentForeground: hsl(h, textSat, 93),
    input: hsl(h, sat, 19),
    surfaceElevated: hsl(h, sat, 13),
    sidebar: hsl(h, sat, 8),
    sidebarForeground: hsl(h, clamp(s - 8, 8, 40), 68),
    sidebarBorder: hsl(h, sat, 15),
  }
}

function buildDarkModeHeaderColor(h: number, s: number, l: number): string {
  const sat = clamp(Math.max(s, 22), 18, 70)
  const headerL = clamp(l - 8, 8, Math.max(l - 1, 8))
  return hsl(h, sat, headerL)
}

function buildDarkModePalette(base: { h: number; s: number; l: number }, brand: string, onDark: string): BrandPalette {
  const { h, s, l } = base
  const sat = clamp(Math.max(s, 22), 18, 70)

  return {
    brand,
    header: buildDarkModeHeaderColor(h, s, l),
    isDarkBrand: true,
    onDark,
    hsl: base,
    darkest: hsl(h, clamp(s + 2, 0, 100), 9),
    accentLine: hsl(h, clamp(s + 18, 0, 100), clamp(base.l + 6, 38, 54)),
    formFrom: hsl(h, clamp(s - 4, 18, 100), 11),
    formTo: hsl(h, clamp(s - 2, 20, 100), 15),
    dark: hsl(h, sat, 13),
    light: hsl(h, clamp(s - 6, 18, 100), 17),
    darkSurfaces: buildBrandDarkSurfaces(h, s),
  }
}

/** Derive header / query-area tones from the admin header_color value. */
export function buildBrandPalette(hex: string | undefined, theme: 'light' | 'dark' = 'light'): BrandPalette {
  const brand = normalizeHex(hex) ?? DEFAULT_BRAND
  const base = hexToHSL(brand) ?? hexToHSL(DEFAULT_BRAND)!
  const isDarkBrand = base.l <= DARK_BRAND_LIGHTNESS_THRESHOLD
  const onDark = base.l > 58 ? '#152033' : '#f8fafc'

  if (theme === 'dark') {
    return buildDarkModePalette(base, brand, onDark)
  }

  if (isDarkBrand) {
    return {
      brand,
      header: brand,
      isDarkBrand,
      onDark,
      hsl: base,
      darkest: hsl(base.h, base.s, clamp(base.l - 16, 8, 18)),
      accentLine: hsl(base.h, clamp(base.s + 22, 0, 100), clamp(base.l + 30, 38, 58)),
      formFrom: hsl(base.h, clamp(base.s + 4, 0, 100), clamp(base.l - 4, 14, 26)),
      formTo: hsl(base.h, clamp(base.s + 8, 0, 100), clamp(base.l + 6, 18, 32)),
      dark: hsl(base.h, base.s, clamp(base.l - 4, 12, 28)),
      light: hsl(base.h, clamp(base.s + 2, 0, 100), clamp(base.l + 4, 16, 30)),
    }
  }

  return {
    brand,
    header: brand,
    isDarkBrand,
    onDark,
    hsl: base,
    darkest: hsl(base.h, clamp(base.s - 4, 0, 100), clamp(base.l - 22, 12, 28)),
    accentLine: hsl(base.h, clamp(base.s + 12, 0, 100), clamp(base.l - 4, 24, 48)),
    formFrom: hsl(base.h, clamp(base.s - 18, 0, 100), clamp(base.l + 38, 88, 97)),
    formTo: hsl(base.h, clamp(base.s - 12, 0, 100), clamp(base.l + 48, 92, 98)),
    dark: hsl(base.h, base.s, clamp(base.l - 12, 18, 34)),
    light: hsl(base.h, clamp(base.s - 20, 0, 100), clamp(base.l + 42, 90, 98)),
  }
}

function clearDarkSurfaces(root: HTMLElement) {
  for (const key of DARK_SURFACE_VARS) {
    root.style.removeProperty(key)
  }
}

function applyDarkSurfaces(root: HTMLElement, surfaces: BrandDarkSurfaces) {
  root.style.setProperty('--color-background', surfaces.background)
  root.style.setProperty('--color-foreground', surfaces.foreground)
  root.style.setProperty('--color-muted', surfaces.muted)
  root.style.setProperty('--color-muted-foreground', surfaces.mutedForeground)
  root.style.setProperty('--color-border', surfaces.border)
  root.style.setProperty('--color-card', surfaces.card)
  root.style.setProperty('--color-card-foreground', surfaces.cardForeground)
  root.style.setProperty('--color-popover', surfaces.popover)
  root.style.setProperty('--color-popover-foreground', surfaces.popoverForeground)
  root.style.setProperty('--color-accent', surfaces.accent)
  root.style.setProperty('--color-accent-foreground', surfaces.accentForeground)
  root.style.setProperty('--color-input', surfaces.input)
  root.style.setProperty('--color-surface-elevated', surfaces.surfaceElevated)
  root.style.setProperty('--color-sidebar', surfaces.sidebar)
  root.style.setProperty('--color-sidebar-foreground', surfaces.sidebarForeground)
  root.style.setProperty('--color-sidebar-border', surfaces.sidebarBorder)
}

export function applyBrandPalette(palette: BrandPalette, theme: 'light' | 'dark' = 'light') {
  const root = document.documentElement
  root.style.setProperty('--brand', palette.brand)
  root.style.setProperty('--brand-header', palette.header)
  root.style.setProperty('--brand-h', String(palette.hsl.h))
  root.style.setProperty('--brand-s', `${palette.hsl.s}%`)
  root.style.setProperty('--brand-l', `${palette.hsl.l}%`)
  root.style.setProperty('--brand-on-dark', palette.onDark)
  root.style.setProperty('--brand-dark', palette.dark)
  root.style.setProperty('--brand-light', palette.light)
  root.style.setProperty('--brand-deepest', palette.darkest)
  root.style.setProperty('--brand-accent-line', palette.accentLine)
  root.style.setProperty('--brand-form-from', palette.formFrom)
  root.style.setProperty('--brand-form-to', palette.formTo)
  root.dataset.brandDark = palette.isDarkBrand ? 'true' : 'false'
  root.dataset.brandLightTone = isDarkBrandColor(palette.brand) ? 'false' : 'true'

  const { h, s } = palette.hsl
  const terminal = buildTerminalColors(h, s, theme)
  root.style.setProperty('--brand-terminal-bg', terminal.bg)
  root.style.setProperty('--brand-terminal-border', terminal.border)
  root.style.setProperty('--brand-terminal-fg', terminal.fg)
  root.style.setProperty('--brand-terminal-muted', terminal.muted)
  root.style.setProperty('--brand-terminal-asn', terminal.asn)

  if (theme === 'dark' && palette.darkSurfaces) {
    applyDarkSurfaces(root, palette.darkSurfaces)
  } else {
    clearDarkSurfaces(root)
  }
}

export function applyBrandColor(hex: string | undefined, theme: 'light' | 'dark' = 'light') {
  const palette = buildBrandPalette(hex, theme)
  applyBrandPalette(palette, theme)
}
