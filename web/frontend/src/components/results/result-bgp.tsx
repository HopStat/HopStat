import { useMemo } from 'react'
import { Star } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useI18n } from '@/contexts/i18n-context'
import { RouteCommunities } from './community-badges'
import { AsPathInline } from './as-path-inline'
import type { ASInfo, BGPRoute, BGPResult } from '@/types/domain'

interface Props {
  result: BGPResult
  enriched: ASInfo[]
}


function routeHasMeta(route: BGPRoute): boolean {
  return Boolean(
    route.via_default_route ||
    (route.communities?.length ?? 0) > 0 ||
    (route.matched_rules?.length ?? 0) > 0,
  )
}

function formatMetric(value: number | undefined): string {
  if (value == null) return '—'
  return String(value)
}

/** Show a weight column only when it helps compare paths (not all zero, not identical on every row). */
function shouldShowRouteMetric(routes: BGPRoute[], key: 'local_pref' | 'med'): boolean {
  if (routes.length === 0) return false
  const values = routes.map(r => r[key] ?? 0)
  if (values.every(v => v === 0)) return false
  const unique = new Set(values)
  return unique.size > 1 || (routes.length === 1 && values[0] > 0)
}

function RouteMetaBlock({ route, compact = false }: { route: BGPRoute; compact?: boolean }) {
  const { t } = useI18n()
  const communities = route.communities ?? []
  const rules = route.matched_rules ?? []
  const hasMeta = routeHasMeta(route)

  if (!hasMeta) return null

  return (
    <div className="space-y-2">
      {route.via_default_route && (
        <Badge variant="secondary" className="text-[10px] normal-case tracking-normal">
          {t('result.via_default_route')}
        </Badge>
      )}
      {(communities.length > 0 || rules.length > 0) && (
        <div className="space-y-1.5">
          {!compact && (
            <span className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
              {t('result.communities')}
            </span>
          )}
          <RouteCommunities communities={communities} rules={rules} />
        </div>
      )}
    </div>
  )
}

function RouteMetricsInline({
  route,
  showLocalPref,
  showMed,
}: {
  route: BGPRoute
  showLocalPref: boolean
  showMed: boolean
}) {
  const { t } = useI18n()
  if (!showLocalPref && !showMed) return null

  const parts: string[] = []
  if (showLocalPref) parts.push(`${t('result.local_pref')} ${formatMetric(route.local_pref)}`)
  if (showMed) parts.push(`${t('result.med')} ${formatMetric(route.med)}`)

  return (
    <div className="font-data text-[10px] tabular-nums text-muted-foreground">
      {parts.join(' · ')}
    </div>
  )
}

function RouteMobileCard({
  route,
  enriched,
  showNode,
  showAsPath,
  showLocalPref,
  showMed,
}: {
  route: BGPRoute
  enriched: ASInfo[]
  showNode: boolean
  showAsPath: boolean
  showLocalPref: boolean
  showMed: boolean
}) {
  const metaLine = showNode ? route.node_name : ''

  return (
    <div className="px-3 py-2.5 space-y-1.5">
      <div className="flex items-start gap-2 min-w-0">
        <div className="w-4 shrink-0 pt-0.5 text-center">
          {route.best ? (
            <Star className="w-3.5 h-3.5 mx-auto fill-amber-400 text-amber-400" aria-hidden />
          ) : (
            <span className="inline-block w-3.5" aria-hidden />
          )}
        </div>
        <div className="min-w-0 flex-1 space-y-1">
          <div className="font-data text-sm font-semibold text-foreground break-all leading-snug">
            {route.prefix || '—'}
          </div>
          {metaLine && (
            <div className="text-[11px] text-muted-foreground leading-snug">{metaLine}</div>
          )}
          {showAsPath && (route.as_path?.length ?? 0) > 0 && (
            <div className="text-xs text-muted-foreground leading-snug">
              <AsPathInline path={route.as_path} enriched={enriched} tooltipPlacement="bottom" />
            </div>
          )}
          <RouteMetricsInline route={route} showLocalPref={showLocalPref} showMed={showMed} />
          <RouteMetaBlock route={route} />
        </div>
      </div>
    </div>
  )
}

export function ResultBGP({ result, enriched }: Props) {
  const { t } = useI18n()
  const routes = useMemo(() => {
    const list = [...(result.routes ?? [])]
    list.sort((a, b) => {
      if (a.best !== b.best) return a.best ? -1 : 1
      return 0
    })
    return list
  }, [result.routes])

  const uniqueNodeNames = useMemo(
    () => new Set(routes.map(r => r.node_name?.trim()).filter(Boolean)),
    [routes],
  )

  if (routes.length === 0) return null

  const showNode = uniqueNodeNames.size > 1
  const showLocalPref = shouldShowRouteMetric(routes, 'local_pref')
  const showMed = shouldShowRouteMetric(routes, 'med')
  const showCommunities = routes.some(routeHasMeta)
  const showMobileAsPath = routes.length > 1

  return (
    <>
      <div className="result-table-frame animate-fade-up md:hidden divide-y divide-border">
        {routes.map((route, i) => (
          <RouteMobileCard
            key={`${route.prefix}-${i}-mobile`}
            route={route}
            enriched={enriched}
            showNode={showNode}
            showAsPath={showMobileAsPath}
            showLocalPref={showLocalPref}
            showMed={showMed}
          />
        ))}
      </div>

      <div className="result-table-frame hidden animate-fade-up md:block">
        <Table className="result-bgp-table md:table" containerClassName="result-table-wrap">
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead className="result-bgp-table__star">
              <span className="sr-only">{t('result.best')}</span>
            </TableHead>
            {showNode && <TableHead className="result-bgp-table__node">{t('result.node')}</TableHead>}
            <TableHead className="result-bgp-table__age hidden lg:table-cell">{t('result.age')}</TableHead>
            <TableHead className="result-bgp-table__prefix">{t('result.prefix')}</TableHead>
            <TableHead className="result-bgp-table__as-path">{t('result.as_path')}</TableHead>
            {showLocalPref && (
              <TableHead className="result-bgp-table__metric">{t('result.local_pref')}</TableHead>
            )}
            {showMed && (
              <TableHead className="result-bgp-table__metric">{t('result.med')}</TableHead>
            )}
            {showCommunities && (
              <TableHead className="result-bgp-table__communities">{t('result.communities')}</TableHead>
            )}
          </TableRow>
        </TableHeader>
        <TableBody>
          {routes.map((route, i) => (
            <TableRow key={`${route.prefix}-${i}`} className="relative hover:z-20">
              <TableCell className="result-bgp-table__star text-center align-middle">
                {route.best ? (
                  <Star
                    className="w-3.5 h-3.5 mx-auto fill-amber-400 text-amber-400"
                    aria-label={t('result.best_route')}
                  />
                ) : null}
              </TableCell>
              {showNode && (
                <TableCell className="result-bgp-table__node text-xs font-medium text-muted-foreground whitespace-nowrap">
                  {route.node_name || '—'}
                </TableCell>
              )}
              <TableCell className="result-bgp-table__age hidden lg:table-cell font-data text-xs text-muted-foreground whitespace-nowrap">
                {route.age || '—'}
              </TableCell>
              <TableCell className="result-bgp-table__prefix font-data text-sm font-medium whitespace-nowrap">
                {route.prefix || '—'}
              </TableCell>
              <TableCell className="result-bgp-table__as-path relative overflow-visible font-data text-xs text-muted-foreground">
                <AsPathInline path={route.as_path} enriched={enriched} tooltipPlacement="bottom" nowrap />
              </TableCell>
              {showLocalPref && (
                <TableCell className="result-bgp-table__metric font-data text-xs tabular-nums text-muted-foreground whitespace-nowrap">
                  {formatMetric(route.local_pref)}
                </TableCell>
              )}
              {showMed && (
                <TableCell className="result-bgp-table__metric font-data text-xs tabular-nums text-muted-foreground whitespace-nowrap">
                  {formatMetric(route.med)}
                </TableCell>
              )}
              {showCommunities && (
                <TableCell className="result-bgp-table__communities align-top py-3">
                  <div className="result-bgp-table__communities-inner min-w-0">
                    <RouteMetaBlock route={route} compact />
                  </div>
                </TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
        </Table>
      </div>
    </>
  )
}
