export const DEFAULT_BRAND = '#1e293b'

const DARK_BRAND_LIGHTNESS_THRESHOLD = 48

/** WCAG 2.1 minimums: normal text, and non-text objects like rings, strokes and icons. */
const AA_TEXT = 4.5
const AA_OBJECT = 3

/** The light-theme surfaces an accent has to stay readable on. Mirrored from the `@theme`
 *  block in globals.css; a test asserts the two never drift apart. */
export const LIGHT_SURFACE_BG = '#f3f5f8'
export const LIGHT_SURFACE_CARD = '#ffffff'

/** Dark-theme surfaces are derived from the brand hue rather than fixed, so the accent has
 *  to be solved against them. These are the lightness stops buildBrandDarkSurfaces uses for
 *  the page and card; keeping them here is what stops the two from drifting. */
const DARK_SURFACE_BG_L = 7
const DARK_SURFACE_CARD_L = 10
const DARK_SURFACE_ELEVATED_L = 13

/** Preferred pair for text on a brand fill — softer than pure black/white, and enough for
 *  every brand colour except a narrow mid band. */
const INK = '#152033'
const PAPER = '#f8fafc'

/** Below this saturation a brand reads as deliberately neutral, and its accent stays
 *  neutral too rather than being pushed into a hue it never had. */
const NEUTRAL_SATURATION = 15

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
  isDarkBrand: boolean
  /** Text and icons on a solid `brand` fill — contrast-picked, always ≥4.5:1. */
  foreground: string
  /** Brand-hued text on the page and card surfaces — ≥4.5:1. */
  accent: string
  /** Brand-hued rings, strokes and icons — ≥3:1. `accentLine` is an alias of this. */
  accentUi: string
  accentLine: string
  /** Secondary text, readable on the brand-tinted band as well as the plain surfaces. */
  muted: string
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
  const r = parseInt(m[1], 16) / 255
  const g = parseInt(m[2], 16) / 255
  const b = parseInt(m[3], 16) / 255
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

export function hexToRgb(hex: string): [number, number, number] | null {
  const normalized = normalizeHex(hex)
  if (!normalized) return null
  return [
    parseInt(normalized.slice(1, 3), 16),
    parseInt(normalized.slice(3, 5), 16),
    parseInt(normalized.slice(5, 7), 16),
  ]
}

export function hslToRgb(h: number, s: number, l: number): [number, number, number] {
  const sat = clamp(s, 0, 100) / 100
  const light = clamp(l, 0, 100) / 100
  const chroma = (1 - Math.abs(2 * light - 1)) * sat
  const sector = ((((h % 360) + 360) % 360) / 60)
  const second = chroma * (1 - Math.abs((sector % 2) - 1))
  const offset = light - chroma / 2

  let base: [number, number, number]
  if (sector < 1) base = [chroma, second, 0]
  else if (sector < 2) base = [second, chroma, 0]
  else if (sector < 3) base = [0, chroma, second]
  else if (sector < 4) base = [0, second, chroma]
  else if (sector < 5) base = [second, 0, chroma]
  else base = [chroma, 0, second]

  return base.map(channel => Math.round((channel + offset) * 255)) as [number, number, number]
}

export function rgbToHex([r, g, b]: [number, number, number]): string {
  const channel = (n: number) => clamp(Math.round(n), 0, 255).toString(16).padStart(2, '0')
  return `#${channel(r)}${channel(g)}${channel(b)}`
}

/** WCAG 2.1 relative luminance. */
export function relativeLuminance([r, g, b]: [number, number, number]): number {
  const linear = (channel: number) => {
    const v = channel / 255
    return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)
  }
  return 0.2126 * linear(r) + 0.7152 * linear(g) + 0.0722 * linear(b)
}

export function contrastRatio(lumA: number, lumB: number): number {
  const hi = Math.max(lumA, lumB)
  const lo = Math.min(lumA, lumB)
  return (hi + 0.05) / (lo + 0.05)
}

function hexLuminance(hex: string): number {
  const rgb = hexToRgb(hex)
  return rgb ? relativeLuminance(rgb) : 0
}

/**
 * Text and icon colour for a solid brand fill. The softer ink/paper pair is preferred and
 * wins for nearly every colour; pure black or white is only reached when the fill sits in
 * the narrow mid band (luminance ≈0.174–0.239) where neither soft tone clears AA.
 *
 * This always has an answer: for any colour, whichever of black and white contrasts better
 * is at least 4.58:1. That is what lets the operator's own hex be kept byte-for-byte — the
 * text moves to meet it, the colour is never darkened to suit the text.
 */
export function pickOnColor(backgroundLum: number): string {
  const better = (candidates: string[]) =>
    candidates.reduce((winner, candidate) =>
      contrastRatio(hexLuminance(candidate), backgroundLum) >
      contrastRatio(hexLuminance(winner), backgroundLum)
        ? candidate
        : winner)

  const soft = better([INK, PAPER])
  if (contrastRatio(hexLuminance(soft), backgroundLum) >= AA_TEXT) return soft
  return better(['#000000', '#ffffff'])
}

/**
 * Walks lightness at a fixed hue and saturation until the colour clears `target` against
 * every surface, preferring the value nearest `start` so the result stays as close to the
 * intended tone as readability allows.
 *
 * This is the piece that replaces hand-picked lightness stops. A stop that reads well for
 * one hue fails for another: at the same HSL lightness a yellow carries far more luminance
 * than a blue, so only measurement generalises.
 */
export function solveLightness(
  h: number,
  sat: number,
  surfaceLums: number[],
  target: number,
  start: number,
): string {
  const clears = (l: number) => {
    const lum = relativeLuminance(hslToRgb(h, sat, l))
    return surfaceLums.every(surface => contrastRatio(lum, surface) >= target)
  }

  for (let offset = 0; offset <= 100; offset++) {
    const darker = start - offset
    if (darker >= 0 && clears(darker)) return rgbToHex(hslToRgb(h, sat, darker))
    const lighter = start + offset
    if (lighter <= 100 && clears(lighter)) return rgbToHex(hslToRgb(h, sat, lighter))
  }

  // Only reachable if the surfaces straddle the middle so far that no single lightness
  // satisfies all of them. Take the end furthest from them rather than returning nothing.
  const onLightSurfaces = surfaceLums.every(lum => lum > 0.5)
  return rgbToHex(hslToRgb(h, sat, onLightSurfaces ? 0 : 100))
}

/**
 * A brand-hued colour that stays readable on the given surfaces, for use as text or as a
 * UI object. Hue is kept so it still reads as the brand; saturation is nudged so a washed
 * out brand still registers as an accent, and lightness is solved.
 */
export function solveReadableAccent(
  h: number,
  s: number,
  surfaceLums: number[],
  target: number,
): string {
  // Lift a washed-out brand enough to still read as an accent rather than as chrome — but
  // never invent colour where there is none: a deliberately neutral brand stays neutral,
  // otherwise a grey would come back red, since grey carries hue 0.
  const sat = s < NEUTRAL_SATURATION ? s : clamp(Math.max(s, 35), 30, 92)
  const onLightSurfaces = surfaceLums.every(lum => lum > 0.5)
  return solveLightness(h, sat, surfaceLums, target, onLightSurfaces ? 40 : 60)
}

/** Saturation buildBrandDarkSurfaces tints its surfaces with. */
function darkSurfaceSaturation(s: number): number {
  return clamp(Math.max(s, 22), 18, 70)
}

/** The stylesheet paints many surfaces as color-mix(in srgb, brand N%, base) — the header
 *  band, the admin sidebar brand block, panels, rows. These are the largest shares it uses.
 *  A dark brand pulls such a surface well past the plain card, which is how secondary text
 *  read at 3.7:1 on the band while passing everywhere else. */
const MAX_TINT_OVER_CARD = 0.3
const MAX_TINT_OVER_BG = 0.24

/** The range is sampled rather than checked at its ends: the surface closest in luminance
 *  to the text — the worst case — can fall in the middle of it. */
const TINT_SAMPLES = 5

function mixSrgb(
  a: [number, number, number],
  b: [number, number, number],
  amountOfA: number,
): [number, number, number] {
  return [
    a[0] * amountOfA + b[0] * (1 - amountOfA),
    a[1] * amountOfA + b[1] * (1 - amountOfA),
    a[2] * amountOfA + b[2] * (1 - amountOfA),
  ]
}

/** Every luminance a brand-tinted surface can take, for the theme's base surfaces. */
export function tintedSurfaceLuminances(
  brand: string,
  h: number,
  s: number,
  theme: 'light' | 'dark',
): number[] {
  const brandRgb = hexToRgb(brand)
  if (!brandRgb) return []

  const sat = darkSurfaceSaturation(s)
  const bases: Array<[[number, number, number], number]> =
    theme === 'dark'
      ? [
          [hslToRgb(h, sat, DARK_SURFACE_CARD_L), MAX_TINT_OVER_CARD],
          [hslToRgb(h, sat, DARK_SURFACE_BG_L), MAX_TINT_OVER_BG],
          [hslToRgb(h, sat, DARK_SURFACE_ELEVATED_L), MAX_TINT_OVER_CARD],
        ]
      : [
          [hexToRgb(LIGHT_SURFACE_CARD) as [number, number, number], MAX_TINT_OVER_CARD],
          [hexToRgb(LIGHT_SURFACE_BG) as [number, number, number], MAX_TINT_OVER_BG],
        ]

  const out: number[] = []
  for (const [base, max] of bases) {
    for (let step = 1; step <= TINT_SAMPLES; step++) {
      out.push(relativeLuminance(mixSrgb(brandRgb, base, (max * step) / TINT_SAMPLES)))
    }
  }
  return out
}

/** The two surface luminances an accent must survive, per theme. */
function surfaceLuminances(h: number, s: number, theme: 'light' | 'dark'): number[] {
  if (theme === 'dark') {
    const sat = darkSurfaceSaturation(s)
    return [
      relativeLuminance(hslToRgb(h, sat, DARK_SURFACE_BG_L)),
      relativeLuminance(hslToRgb(h, sat, DARK_SURFACE_CARD_L)),
    ]
  }
  return [hexLuminance(LIGHT_SURFACE_BG), hexLuminance(LIGHT_SURFACE_CARD)]
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

  // Text on the console is solved against its own background rather than placed at a fixed
  // lightness. The stops that used to be here read fine for a blue and failed badly for a
  // yellow, because equal HSL lightness is not equal luminance.
  if (theme === 'dark') {
    const bgLum = relativeLuminance(hslToRgb(h, sat, 8))
    return {
      bg: hsl(h, sat, 8),
      border: hsl(h, sat, 16),
      fg: solveLightness(h, clamp(s - 20, 6, 30), [bgLum], AA_TEXT, 82),
      muted: solveLightness(h, clamp(s - 16, 8, 36), [bgLum], AA_TEXT, 52),
      asn: solveLightness(h, clamp(s + 10, 42, 78), [bgLum], AA_TEXT, 72),
    }
  }

  const lightSat = clamp(s - 24, 6, 28)
  const asnSat = clamp(Math.max(s, 38), 42, 72)
  const bgLum = relativeLuminance(hslToRgb(h, lightSat, 97))
  return {
    bg: hsl(h, lightSat, 97),
    border: hsl(h, lightSat, 88),
    fg: solveLightness(h, clamp(s - 8, 14, 38), [bgLum], AA_TEXT, 24),
    muted: solveLightness(h, clamp(s - 12, 10, 32), [bgLum], AA_TEXT, 46),
    asn: solveLightness(h, asnSat, [bgLum], AA_TEXT, 36),
  }
}

export function isDarkBrandColor(hex: string): boolean {
  const parsed = hexToHSL(hex || DEFAULT_BRAND)
  return parsed != null && parsed.l <= DARK_BRAND_LIGHTNESS_THRESHOLD
}

/** Brand-tinted page surfaces for dark mode. */
export function buildBrandDarkSurfaces(h: number, s: number, brand: string): BrandDarkSurfaces {
  const sat = darkSurfaceSaturation(s)
  const textSat = clamp(s - 22, 6, 34)

  // Secondary text and the input boundary are solved against the surfaces they land on.
  // Both used to sit at fixed lightness: mutedForeground missed 4.5:1 on light-luminance
  // hues, and the input boundary — the only affordance marking where a field is, since its
  // fill barely differs from the card — missed 3:1 for every hue.
  const cardLum = relativeLuminance(hslToRgb(h, sat, DARK_SURFACE_CARD_L))
  const bgLum = relativeLuminance(hslToRgb(h, sat, DARK_SURFACE_BG_L))
  const mutedLum = relativeLuminance(hslToRgb(h, sat, 11))
  const elevatedLum = relativeLuminance(hslToRgb(h, sat, DARK_SURFACE_ELEVATED_L))
  const textSurfaces = [bgLum, cardLum, mutedLum, elevatedLum, ...tintedSurfaceLuminances(brand, h, s, 'dark')]

  return {
    background: hsl(h, sat, DARK_SURFACE_BG_L),
    foreground: hsl(h, textSat, 93),
    muted: hsl(h, sat, 11),
    mutedForeground: solveLightness(h, clamp(s - 12, 8, 42), textSurfaces, AA_TEXT, 58),
    border: hsl(h, sat, 17),
    card: hsl(h, sat, DARK_SURFACE_CARD_L),
    cardForeground: hsl(h, textSat, 93),
    popover: hsl(h, sat, 11),
    popoverForeground: hsl(h, textSat, 93),
    accent: hsl(h, sat, 14),
    accentForeground: hsl(h, textSat, 93),
    input: solveLightness(h, sat, [cardLum], AA_OBJECT, 19),
    surfaceElevated: hsl(h, sat, DARK_SURFACE_ELEVATED_L),
    sidebar: hsl(h, sat, 8),
    sidebarForeground: hsl(h, clamp(s - 8, 8, 40), 68),
    sidebarBorder: hsl(h, sat, 15),
  }
}

/** Muted text keeps its low-saturation, secondary tone; only its lightness is solved. */
const MUTED_SATURATION = 15

/**
 * Secondary text that stays readable on the brand-tinted band as well as on the plain
 * surfaces. --color-muted-foreground is a fixed value, so it cannot follow a surface that
 * the brand darkens; this can.
 */
export function solveMutedForeground(
  h: number,
  s: number,
  surfaceLums: number[],
  theme: 'light' | 'dark',
): string {
  const sat = Math.min(s, MUTED_SATURATION)
  return solveLightness(h, sat, surfaceLums, AA_TEXT, theme === 'dark' ? 62 : 42)
}

interface ContrastTokens {
  foreground: string
  accent: string
  accentUi: string
  muted: string
}

/**
 * The three colours that have to be measured rather than guessed. Solved per theme,
 * because in dark mode the surfaces an accent sits on are themselves derived from the
 * brand hue — so the surfaces must exist before the accent can be solved against them.
 */
function contrastTokens(brand: string, h: number, s: number, theme: 'light' | 'dark'): ContrastTokens {
  const surfaces = surfaceLuminances(h, s, theme)
  return {
    foreground: pickOnColor(hexLuminance(brand)),
    accent: solveReadableAccent(h, s, surfaces, AA_TEXT),
    accentUi: solveReadableAccent(h, s, surfaces, AA_OBJECT),
    muted: solveMutedForeground(h, s, [...surfaces, ...tintedSurfaceLuminances(brand, h, s, theme)], theme),
  }
}

function buildDarkModePalette(
  base: { h: number; s: number; l: number },
  brand: string,
  contrast: ContrastTokens,
): BrandPalette {
  const { h, s } = base

  return {
    brand,
    isDarkBrand: true,
    foreground: contrast.foreground,
    accent: contrast.accent,
    accentUi: contrast.accentUi,
    hsl: base,
    accentLine: contrast.accentUi,
    muted: contrast.muted,
    darkSurfaces: buildBrandDarkSurfaces(h, s, brand),
  }
}

/** Derive header / query-area tones from the admin header_color value. */
export function buildBrandPalette(hex: string | undefined, theme: 'light' | 'dark' = 'light'): BrandPalette {
  const brand = normalizeHex(hex) ?? DEFAULT_BRAND
  const base = hexToHSL(brand) ?? hexToHSL(DEFAULT_BRAND)!
  const isDarkBrand = base.l <= DARK_BRAND_LIGHTNESS_THRESHOLD
  const contrast = contrastTokens(brand, base.h, base.s, theme)

  if (theme === 'dark') {
    return buildDarkModePalette(base, brand, contrast)
  }

  if (isDarkBrand) {
    return {
      brand,
      isDarkBrand,
      foreground: contrast.foreground,
      accent: contrast.accent,
      accentUi: contrast.accentUi,
      hsl: base,
      accentLine: contrast.accentUi,
    muted: contrast.muted,
    }
  }

  return {
    brand,
    isDarkBrand,
    foreground: contrast.foreground,
    accent: contrast.accent,
    accentUi: contrast.accentUi,
    hsl: base,
    accentLine: contrast.accentUi,
    muted: contrast.muted,
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
  root.style.setProperty('--brand-h', String(palette.hsl.h))
  root.style.setProperty('--brand-s', `${palette.hsl.s}%`)
  root.style.setProperty('--brand-l', `${palette.hsl.l}%`)
  root.style.setProperty('--brand-foreground', palette.foreground)
  root.style.setProperty('--brand-accent', palette.accent)
  root.style.setProperty('--brand-accent-ui', palette.accentUi)
  root.style.setProperty('--brand-accent-line', palette.accentLine)
  root.style.setProperty('--brand-muted', palette.muted)
  root.dataset.brandDark = palette.isDarkBrand ? 'true' : 'false'

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
