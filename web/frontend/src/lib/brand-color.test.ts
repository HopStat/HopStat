import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import {
  buildBrandDarkSurfaces,
  buildBrandPalette,
  contrastRatio,
  hexToHSL,
  hexToRgb,
  hslToRgb,
  LIGHT_SURFACE_BG,
  LIGHT_SURFACE_CARD,
  normalizeHex,
  relativeLuminance,
} from './brand-color'

const AA_TEXT = 4.5
const AA_OBJECT = 3

const lumOfHex = (hex: string) => relativeLuminance(hexToRgb(hex)!)

/** Dark surfaces are emitted as `hsl(h s% l%)` strings, so read them back to measure. */
function lumOfHslString(value: string): number {
  const m = value.match(/hsl\((\d+(?:\.\d+)?) (\d+(?:\.\d+)?)% (\d+(?:\.\d+)?)%\)/)
  if (!m) throw new Error(`not an hsl() string: ${value}`)
  return relativeLuminance(hslToRgb(Number(m[1]), Number(m[2]), Number(m[3])))
}

/** The brand colours a real deployment is most likely to break on. */
const NOTABLE = [
  '#e0edd4', // the pastel green that started this — white on it is 1.22:1
  '#1e293b', // the seeded default
  '#ffff00', // maximum luminance at full saturation
  '#808080', // mid grey, the band where the soft ink/paper pair gives up
  '#000000',
  '#ffffff',
]

describe('colour maths', () => {
  it('measures relative luminance and contrast the way WCAG does', () => {
    expect(lumOfHex('#ffffff')).toBeCloseTo(1, 5)
    expect(lumOfHex('#000000')).toBeCloseTo(0, 5)
    expect(lumOfHex('#e0edd4')).toBeCloseTo(0.8117, 3)
    expect(contrastRatio(lumOfHex('#ffffff'), lumOfHex('#000000'))).toBeCloseTo(21, 5)
  })

  it('pins the bug: white on the reported brand is unreadable', () => {
    expect(contrastRatio(lumOfHex('#ffffff'), lumOfHex('#e0edd4'))).toBeLessThan(1.3)
  })

  it('round-trips hsl and hex', () => {
    expect(hslToRgb(0, 0, 100)).toEqual([255, 255, 255])
    expect(hslToRgb(0, 100, 50)).toEqual([255, 0, 0])
    expect(hslToRgb(120, 100, 50)).toEqual([0, 255, 0])
    expect(hslToRgb(240, 100, 50)).toEqual([0, 0, 255])
  })
})

describe('the operator’s colour is never altered', () => {
  it.each(NOTABLE)('keeps %s byte-for-byte', hex => {
    for (const theme of ['light', 'dark'] as const) {
      expect(buildBrandPalette(hex, theme).brand).toBe(normalizeHex(hex))
    }
  })
})

// A grid rather than a handful of samples: the whole point of solving contrast instead of
// clamping lightness is that it has to hold for every hue, and a fixed clamp passes some
// hues by luck. ~2600 colours keeps the run under a second.
const GRID: Array<{ hex: string; h: number; s: number; l: number }> = []
for (let h = 0; h < 360; h += 10) {
  for (const s of [0, 25, 50, 75, 100]) {
    for (let l = 0; l <= 100; l += 5) {
      GRID.push({ hex: `#${hslToRgb(h, s, l).map(c => c.toString(16).padStart(2, '0')).join('')}`, h, s, l })
    }
  }
}

describe('every brand colour resolves to a readable set', () => {
  it('puts readable text on a solid brand fill, in both themes', () => {
    const failures = GRID.filter(({ hex }) =>
      (['light', 'dark'] as const).some(theme => {
        const palette = buildBrandPalette(hex, theme)
        return contrastRatio(lumOfHex(palette.foreground), lumOfHex(palette.brand)) < AA_TEXT
      }))
    expect(failures).toEqual([])
  })

  it('keeps brand-hued text readable on the light surfaces', () => {
    const surfaces = [lumOfHex(LIGHT_SURFACE_BG), lumOfHex(LIGHT_SURFACE_CARD)]
    const failures = GRID.filter(({ hex }) => {
      const accent = lumOfHex(buildBrandPalette(hex, 'light').accent)
      return surfaces.some(surface => contrastRatio(accent, surface) < AA_TEXT)
    })
    expect(failures).toEqual([])
  })

  it('keeps brand-hued rings and strokes visible on the light surfaces', () => {
    const surfaces = [lumOfHex(LIGHT_SURFACE_BG), lumOfHex(LIGHT_SURFACE_CARD)]
    const failures = GRID.filter(({ hex }) => {
      const accentUi = lumOfHex(buildBrandPalette(hex, 'light').accentUi)
      return surfaces.some(surface => contrastRatio(accentUi, surface) < AA_OBJECT)
    })
    expect(failures).toEqual([])
  })

  // Dark surfaces are themselves derived from the brand hue, so the accent has to be solved
  // against them rather than against fixed values. If someone reorders buildBrandPalette so
  // the accents are solved before the surfaces exist, this is what catches it.
  it('solves accents against the derived dark surfaces, not fixed ones', () => {
    const failures = GRID.filter(({ hex }) => {
      const palette = buildBrandPalette(hex, 'dark')
      // Read the hue and saturation back off the palette rather than the grid: a hex is
      // only 8 bits per channel, so a pale colour does not round-trip to the saturation it
      // was generated from, and the surfaces have to be the ones the palette actually used.
      const { h, s } = palette.hsl
      const surfaces = [
        lumOfHslString(buildBrandDarkSurfaces(h, s, palette.brand).background),
        lumOfHslString(buildBrandDarkSurfaces(h, s, palette.brand).card),
      ]
      const accent = lumOfHex(palette.accent)
      const accentUi = lumOfHex(palette.accentUi)
      return surfaces.some(surface =>
        contrastRatio(accent, surface) < AA_TEXT || contrastRatio(accentUi, surface) < AA_OBJECT)
    })
    expect(failures).toEqual([])
  })

  it('keeps the accent on the brand’s own hue when the brand has one', () => {
    const saturated = GRID.filter(({ s, l }) => s >= 50 && l > 10 && l < 90)
    const drifted = saturated.filter(({ hex, h }) => {
      const accentHue = hexToHSL(buildBrandPalette(hex, 'light').accent)!.h
      // Shortest way round the colour wheel, so 359° and 1° count as two degrees apart.
      const delta = Math.abs(((accentHue - h + 540) % 360) - 180)
      return delta > 2
    })
    expect(drifted).toEqual([])
  })

  it('leaves a deliberately neutral brand neutral instead of inventing a hue', () => {
    const accent = buildBrandPalette('#808080', 'light').accent
    const rgb = hexToRgb(accent)!
    expect(Math.max(...rgb) - Math.min(...rgb)).toBeLessThanOrEqual(8)
  })
})

describe('solved values for the colours that matter', () => {
  it('reads dark ink on the pastel green that reported the bug', () => {
    const palette = buildBrandPalette('#e0edd4', 'light')
    expect(palette.foreground).toBe('#152033')
    expect(contrastRatio(lumOfHex(palette.foreground), lumOfHex(palette.brand))).toBeGreaterThan(13)
  })

  it('reads pale paper on the seeded navy', () => {
    expect(buildBrandPalette('#1e293b', 'light').foreground).toBe('#f8fafc')
  })

  // Mid-luminance fills are the one case the softer ink/paper pair cannot serve.
  it('escalates to pure black or white only in the mid band', () => {
    expect(buildBrandPalette('#808080', 'light').foreground).toBe('#000000')
  })

  it('aliases accentLine onto the object-contrast accent, so its call sites inherit the fix', () => {
    for (const hex of NOTABLE) {
      for (const theme of ['light', 'dark'] as const) {
        const palette = buildBrandPalette(hex, theme)
        expect(palette.accentLine).toBe(palette.accentUi)
      }
    }
  })
})

describe('the light surface constants match the stylesheet', () => {
  it('does not drift from the @theme block', () => {
    // Read the file rather than importing it: the Tailwind plugin compiles CSS imports, so
    // even `?raw` comes back empty here.
    const css = readFileSync('src/globals.css', 'utf8')
    const theme = css.slice(css.indexOf('@theme'), css.indexOf('@layer base'))
    expect(theme).toContain(`--color-background: ${LIGHT_SURFACE_BG};`)
    expect(theme).toContain(`--color-card: ${LIGHT_SURFACE_CARD};`)
  })
})
