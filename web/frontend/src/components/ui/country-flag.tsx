import { cn } from '@/lib/utils'

interface Props {
  code: string
  className?: string
  width?: number
  height?: number
}

/** Small flag image (PNG) — reliable on Windows where Unicode flag emoji shows letter boxes. */
export function CountryFlag({ code, className, width = 16, height = 12 }: Props) {
  const cc = code.trim().toLowerCase()
  if (!/^[a-z]{2}$/.test(cc)) return null

  return (
    <img
      src={`https://flagcdn.com/16x12/${cc}.png`}
      srcSet={`https://flagcdn.com/32x24/${cc}.png 2x`}
      width={width}
      height={height}
      alt=""
      aria-hidden
      loading="lazy"
      decoding="async"
      className={cn('inline-block shrink-0 rounded-[2px] object-cover shadow-sm', className)}
    />
  )
}
