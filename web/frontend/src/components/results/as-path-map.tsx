import { Fragment } from 'react'
import { ArrowDown, ArrowRight } from 'lucide-react'
import { CountryFlag } from '@/components/ui/country-flag'
import { AsPathTooltip } from '@/components/results/as-path-tooltip'
import { useI18n } from '@/contexts/i18n-context'
import { asCountryCode, asTooltipLines, buildAsInfoMap, compressConsecutiveASPath, displayAsName, formatASLabel } from '@/lib/as-info'
import type { ASInfo, BGPRoute } from '@/types/domain'

interface Props {
  routes: BGPRoute[]
  enriched: ASInfo[]
}

function pillClass(index: number, total: number): string {
  if (index === 0) return 'bg-brand'
  if (index === total - 1) return 'bg-slate-700 dark:bg-slate-600'
  return 'bg-slate-500 dark:bg-slate-600'
}

export function AsPathMap({ routes, enriched }: Props) {
  const { locale } = useI18n()
  const asPath = routes?.[0]?.as_path ?? []
  const prefix = routes?.[0]?.prefix?.trim()
  if (asPath.length === 0) return null

  const byAsn = buildAsInfoMap(enriched)
  const hops = compressConsecutiveASPath(asPath)
  if (hops.length === 0) return null

  return (
    <div className="result-surface result-surface--path px-3 py-3 sm:px-5 sm:py-4">
      {prefix && (
        <p className="font-data text-sm sm:text-base font-bold text-foreground mb-2.5 tracking-tight">
          {prefix}
        </p>
      )}

      <div className="result-surface--path__scroll">
        <div className="result-surface--path__lane">
          <div className="result-surface--path__hops">
          {hops.map(({ asn, count }, i) => {
            const asnNum = Number(asn)
            const info = byAsn.get(asnNum)
            const name = displayAsName(info)
            const cc = asCountryCode(info)
            const tooltipLines = asTooltipLines(info, asn, count, locale)

            return (
              <Fragment key={`${asn}-${i}`}>
                {i > 0 && (
                  <>
                    <ArrowDown className="w-4 h-4 text-muted-foreground/45 shrink-0 sm:hidden" aria-hidden />
                    <ArrowRight className="hidden sm:block w-4 h-4 text-muted-foreground/45 shrink-0 mb-6" aria-hidden />
                  </>
                )}
                <div className="result-surface--path__hop">
                  <AsPathTooltip lines={tooltipLines} placement="top" className="flex w-full">
                    <div
                      className={`w-full rounded-md px-1.5 sm:px-2 py-1.5 sm:py-2 text-center text-white font-semibold text-[11px] sm:text-sm cursor-pointer ${pillClass(i, hops.length)}`}
                    >
                      {formatASLabel(asn, count)}
                    </div>
                  </AsPathTooltip>

                  <div className="flex w-full flex-col items-center gap-0.5 min-h-[1.25rem] px-0.5 min-w-0">
                    {(name || cc) ? (
                      <div className="flex items-center justify-center gap-1 w-full min-w-0">
                        {cc && <CountryFlag code={cc} />}
                        {name && (
                          <span className="as-map-org-label min-w-0 max-w-full truncate text-center text-foreground/75">
                            {name}
                          </span>
                        )}
                      </div>
                    ) : (
                      <span className="text-[10px] text-muted-foreground/50">…</span>
                    )}
                  </div>
                </div>
              </Fragment>
            )
          })}
          </div>
        </div>
      </div>
    </div>
  )
}
