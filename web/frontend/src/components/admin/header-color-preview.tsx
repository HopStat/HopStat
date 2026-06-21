import { useTheme } from '@/contexts/theme-context'
import { buildBrandPalette } from '@/lib/brand-color'

interface Props {
  color: string
}

function headerBandBackground(palette: ReturnType<typeof buildBrandPalette>): string {
  return palette.header
}

export function HeaderColorPreview({ color }: Props) {
  const { theme } = useTheme()
  const palette = buildBrandPalette(color, theme)

  return (
    <div
      className="ml-auto h-10 flex-1 max-w-[180px] rounded-xl overflow-hidden ring-1 ring-border/80"
      title={palette.brand}
    >
      <div className="h-[42%]" style={{ background: headerBandBackground(palette) }} />
      <div
        className="h-px"
        style={{ background: palette.accentLine, opacity: 0.75 }}
      />
      <div
        className="h-[calc(58%-1px)] flex items-end p-1.5"
        style={{ background: `linear-gradient(180deg, ${palette.formTo}, ${palette.formFrom})` }}
      >
        <div className="h-2 w-full rounded-sm bg-white/90 ring-1 ring-black/5" />
      </div>
    </div>
  )
}
