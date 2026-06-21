import { Fragment, useMemo } from 'react'
import { AsPathTooltip } from '@/components/results/as-path-tooltip'
import { buildAsInfoMap, asTooltipLines, compressConsecutiveASPath, formatASLabel } from '@/lib/as-info'
import { useI18n } from '@/contexts/i18n-context'
import type { ASInfo } from '@/types/domain'

interface Props {
  path: number[] | undefined
  enriched: ASInfo[]
  tooltipPlacement?: 'top' | 'bottom'
  nowrap?: boolean
}

export function AsPathInline({ path, enriched, tooltipPlacement = 'top', nowrap = false }: Props) {
  const { locale } = useI18n()
  const byAsn = useMemo(() => buildAsInfoMap(enriched), [enriched])
  const hops = useMemo(() => compressConsecutiveASPath(path ?? []), [path])

  if (hops.length === 0) return <>—</>

  return (
    <span className={`inline-flex items-center gap-x-0.5 ${nowrap ? 'flex-nowrap whitespace-nowrap' : 'flex-wrap gap-y-0.5'}`}>
      {hops.map(({ asn, count }, i) => {
        const info = byAsn.get(asn)
        const tooltipLines = asTooltipLines(info, asn, count, locale)

        return (
          <Fragment key={`${asn}-${i}`}>
            {i > 0 && <span className="text-muted-foreground/50 px-0.5">→</span>}
            <AsPathTooltip lines={tooltipLines} placement={tooltipPlacement}>
              <span className="cursor-pointer rounded px-0.5 hover:bg-muted/80 hover:text-foreground transition-colors">
                {formatASLabel(asn, count)}
              </span>
            </AsPathTooltip>
          </Fragment>
        )
      })}
    </span>
  )
}
