import { useTheme } from '@/contexts/theme-context'
import { buildBrandPalette, contrastRatio, hexToRgb, relativeLuminance } from '@/lib/brand-color'

interface Props {
  color: string
}

function ratioOf(a: string, b: string): number {
  const [rgbA, rgbB] = [hexToRgb(a), hexToRgb(b)]
  if (!rgbA || !rgbB) return 0
  return contrastRatio(relativeLuminance(rgbA), relativeLuminance(rgbB))
}

/**
 * Shows what the chosen colour will actually look like in use. The old preview painted
 * three palette fields that no stylesheet consumed, so it could look fine while the real
 * site did not — the point of showing a filled button with real text on it is that a pale
 * colour proves itself here rather than in production.
 */
export function HeaderColorPreview({ color }: Props) {
  const { theme } = useTheme()
  const palette = buildBrandPalette(color, theme)
  const onBrand = ratioOf(palette.foreground, palette.brand)

  return (
    <div className="ml-auto flex items-center gap-3">
      <div
        className="flex h-10 items-center gap-2 rounded-md px-3"
        style={{ background: palette.brand, color: palette.foreground }}
        title={`${palette.brand} · ${onBrand.toFixed(1)}:1`}
      >
        <span className="text-xs font-semibold">Aa</span>
        <span className="h-4 w-px opacity-30" style={{ background: palette.foreground }} />
        <span className="font-data text-2xs tabular-nums">{onBrand.toFixed(1)}:1</span>
      </div>
      <span
        className="font-data text-2xs"
        style={{ color: palette.accent }}
        title={`Link and accent tone: ${palette.accent}`}
      >
        AS13335
      </span>
    </div>
  )
}
