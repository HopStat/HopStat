import { describe, expect, it } from 'vitest'
import {
  tintedSurfaceLuminances,
  buildBrandDarkSurfaces,
  buildBrandPalette,
  buildTerminalColors,
  contrastRatio,
  hexToRgb,
  hslToRgb,
  LIGHT_SURFACE_BG,
  LIGHT_SURFACE_CARD,
  relativeLuminance,
} from './brand-color'

const AA_TEXT = 4.5
const AA_OBJECT = 3

/**
 * The palette emits two colour notations: contrast-solved tokens as hex, and the older
 * derived surfaces as `hsl(h s% l%)` strings. Both have to be measurable, because the
 * stylesheet composes them against each other.
 */
function toRgb(color: string): [number, number, number] {
  const hex = hexToRgb(color)
  if (hex) return hex
  const m = color.match(/hsl\(\s*([\d.]+)\s+([\d.]+)%\s+([\d.]+)%\s*\)/)
  if (!m) throw new Error(`unrecognised colour: ${color}`)
  return hslToRgb(Number(m[1]), Number(m[2]), Number(m[3]))
}

const ratio = (a: string, b: string) =>
  contrastRatio(relativeLuminance(toRgb(a)), relativeLuminance(toRgb(b)))

interface Failure {
  brand: string
  theme: string
  pair: string
  ratio: number
  need: number
}

/** Every foreground/background pair the stylesheet actually puts together. */
function checkPalette(brand: string, theme: 'light' | 'dark'): Failure[] {
  const palette = buildBrandPalette(brand, theme)
  const { h, s } = palette.hsl
  const terminal = buildTerminalColors(h, s, theme)
  const failures: Failure[] = []

  const check = (pair: string, fg: string, bg: string, need: number) => {
    const value = ratio(fg, bg)
    if (value < need) failures.push({ brand, theme, pair, ratio: +value.toFixed(2), need })
  }

  // Text on a solid brand fill, and the brand used as text or as a UI object.
  check('foreground on brand', palette.foreground, palette.brand, AA_TEXT)

  const surfaces: Array<[string, string]> =
    theme === 'dark'
      ? (() => {
          const d = buildBrandDarkSurfaces(h, s, brand)
          return [
            ['background', d.background],
            ['card', d.card],
          ]
        })()
      : [
          ['background', LIGHT_SURFACE_BG],
          ['card', LIGHT_SURFACE_CARD],
        ]

  for (const [name, surface] of surfaces) {
    check(`accent on ${name}`, palette.accent, surface, AA_TEXT)
    check(`accentUi on ${name}`, palette.accentUi, surface, AA_OBJECT)
  }

  // The dark chrome is derived from the brand hue by fixed lightness stops rather than by
  // solving for contrast, so these pairs are the ones most likely to miss.
  if (theme === 'dark') {
    const d = buildBrandDarkSurfaces(h, s, brand)
    check('foreground on background', d.foreground, d.background, AA_TEXT)
    check('foreground on card', d.cardForeground, d.card, AA_TEXT)
    check('foreground on muted', d.foreground, d.muted, AA_TEXT)
    check('foreground on elevated', d.foreground, d.surfaceElevated, AA_TEXT)
    check('mutedForeground on background', d.mutedForeground, d.background, AA_TEXT)
    check('mutedForeground on card', d.mutedForeground, d.card, AA_TEXT)
    check('popoverForeground on popover', d.popoverForeground, d.popover, AA_TEXT)
    check('accentForeground on accent', d.accentForeground, d.accent, AA_TEXT)
    check('sidebarForeground on sidebar', d.sidebarForeground, d.sidebar, AA_TEXT)
    // --color-input draws the boundary of text inputs and selects, which 1.4.11 counts as
    // a user-interface component. --color-border and --color-sidebar-border only draw
    // dividers and card outlines, which the same criterion exempts as decorative — holding
    // those to 3:1 would fail every design system and teach us to ignore the report.
    check('input boundary on card', d.input, d.card, AA_OBJECT)
  }

  // The terminal paints its own surface in both themes. Its border is a decorative frame,
  // so only the text is measured.
  check('terminal fg on terminal bg', terminal.fg, terminal.bg, AA_TEXT)
  check('terminal asn on terminal bg', terminal.asn, terminal.bg, AA_TEXT)
  check('terminal muted on terminal bg', terminal.muted, terminal.bg, AA_TEXT)

  return failures
}

/** The admin can pick any hex, so the whole space has to hold — not a handful of samples. */
const BRANDS: string[] = []
for (let h = 0; h < 360; h += 15) {
  for (const s of [0, 8, 20, 40, 60, 80, 100]) {
    for (let l = 5; l <= 95; l += 10) {
      const [r, g, b] = hslToRgb(h, s, l)
      BRANDS.push(`#${[r, g, b].map(c => c.toString(16).padStart(2, '0')).join('')}`)
    }
  }
}

function report(failures: Failure[]): string {
  const byPair = new Map<string, Failure[]>()
  for (const f of failures) {
    const list = byPair.get(f.pair) ?? []
    list.push(f)
    byPair.set(f.pair, list)
  }
  const lines = [`${failures.length} pair(s) below threshold across ${BRANDS.length} brand colours:`]
  for (const [pair, list] of [...byPair.entries()].sort((a, b) => b[1].length - a[1].length)) {
    const worst = list.reduce((a, b) => (a.ratio < b.ratio ? a : b))
    lines.push(
      `  ${pair} — ${list.length} brands, worst ${worst.ratio}:1 (need ${worst.need}) at ${worst.brand} ${worst.theme}`,
    )
  }
  return lines.join('\n')
}

describe('every admin-chosen brand keeps the whole palette readable', () => {
  it.each(['light', 'dark'] as const)('holds in %s mode', theme => {
    const failures = BRANDS.flatMap(brand => checkPalette(brand, theme))
    expect(failures.length === 0 ? '' : report(failures)).toBe('')
  })
})

describe('the static tokens in globals.css are readable too', () => {
  // The brand engine cannot vouch for hand-written token values, and those carry most of
  // the chrome. Pairs are taken from how the stylesheet composes them.
  const staticPairs: Array<[string, string, string, number]> = [
    ['light body text', '#152033', '#f3f5f8', AA_TEXT],
    ['light muted text', '#5a677a', '#f3f5f8', AA_TEXT],
    ['light muted text on card', '#5a677a', '#ffffff', AA_TEXT],
    ['light input boundary', '#798eaa', '#ffffff', AA_OBJECT],
    ['dark input boundary', '#52678a', '#121820', AA_OBJECT],
    ['dark body text', '#e8edf4', '#0c1017', AA_TEXT],
    ['dark muted text', '#8b97a8', '#0c1017', AA_TEXT],
    ['dark muted text on card', '#8b97a8', '#121820', AA_TEXT],
    ['dark sidebar text', '#a8b4c4', '#0f141c', AA_TEXT],
  ]

  it.each(staticPairs)('%s clears its threshold', (_name, fg, bg, need) => {
    expect(ratio(fg, bg)).toBeGreaterThanOrEqual(need)
  })
})

describe('secondary text survives every brand-tinted surface', () => {
  // The header band and the admin sidebar brand block mix the brand into the card. A dark
  // brand darkens that surface well past the plain card, which is how muted text on it read
  // at 3.7:1 while passing everywhere else. Asserted here so it cannot come back silently.
  it.each(['light', 'dark'] as const)('holds in %s mode', theme => {
    const failures: string[] = []
    for (const brand of BRANDS) {
      const palette = buildBrandPalette(brand, theme)
      const { h, s } = palette.hsl
      // Both secondary-text tokens: the brand-derived one, and the dark theme's own.
      const secondary = [palette.muted]
      if (theme === 'dark' && palette.darkSurfaces) secondary.push(palette.darkSurfaces.mutedForeground)

      for (const surface of tintedSurfaceLuminances(palette.brand, h, s, theme)) {
        for (const text of secondary) {
          const got = contrastRatio(relativeLuminance(toRgb(text)), surface)
          if (got < AA_TEXT) {
            failures.push(`${brand} ${theme}: ${text} on tinted surface = ${got.toFixed(2)}:1`)
          }
        }
      }
    }
    expect(failures.slice(0, 10).join('\n')).toBe('')
  })
})
