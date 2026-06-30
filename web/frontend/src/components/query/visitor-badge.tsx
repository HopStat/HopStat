import { useEffect, useState } from 'react'
import { MapPin } from 'lucide-react'
import { api } from '@/lib/api-client'
import { CountryFlag } from '@/components/ui/country-flag'
import { cn } from '@/lib/utils'
import type { MyIPResult } from '@/types/domain'

interface Props {
  onTraceroute: (ip: string) => void
  className?: string
}

function displayISP(org: string): string {
  const trimmed = org.trim()
  if (!trimmed) return ''
  return trimmed.split(/\s+/)[0] ?? trimmed
}

export function VisitorBadge({ onTraceroute, className }: Props) {
  const [info, setInfo] = useState<MyIPResult | null>(null)

  useEffect(() => {
    api.get<MyIPResult>('/myip').then(setInfo).catch(() => {})
  }, [])

  if (!info?.ip) return null

  const isp = displayISP(info.asn_org ?? '')
  const city = info.city?.trim() ?? ''
  const cc = info.country_code?.trim().toLocaleUpperCase('en-US') ?? ''
  const titleParts = [info.ip, isp, city, info.country?.trim() || cc].filter(Boolean)

  return (
    <button
      type="button"
      onClick={() => onTraceroute(info.ip)}
      title={titleParts.join(' • ')}
      className={cn(
        'visitor-ip-badge inline-flex w-fit max-w-full h-7 items-center justify-center gap-1.5 rounded-md px-2.5 text-xs font-semibold leading-none',
        className,
      )}
    >
      <MapPin className="visitor-ip-badge__icon w-3.5 h-3.5 shrink-0" />
      <span className="inline-flex min-w-0 items-center justify-center gap-x-1 font-mono">
        <span className="visitor-ip-badge__ip shrink-0 whitespace-nowrap">{info.ip}</span>
        {(isp || city) && (
          <span className="visitor-ip-badge__meta hidden sm:inline-flex items-center gap-x-1">
            {isp && <span className="whitespace-nowrap">{isp}</span>}
            {isp && city && <span className="shrink-0">•</span>}
            {city && <span className="whitespace-nowrap">{city}</span>}
          </span>
        )}
      </span>
      {cc && (
        <CountryFlag
          code={cc}
          width={16}
          height={11}
          className="visitor-ip-badge__flag rounded-[1px] shadow-none shrink-0 w-[1.35em] h-[0.95em] object-cover"
        />
      )}
    </button>
  )
}
