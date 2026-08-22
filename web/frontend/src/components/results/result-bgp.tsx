import { useMemo, type ReactNode } from 'react'
import { Star } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useI18n } from '@/contexts/i18n-context'
import { RouteCommunities } from './community-badges'
import { AsPathInline } from './as-path-inline'
import { CopyResultButton } from './copy-result-button'
import { isTextSelectionClick, useCopyFeedback } from '@/hooks/use-copy-feedback'
import type { ASInfo, BGPRoute, BGPResult } from '@/types/domain'

interface Props {
  result: BGPResult
  enriched: ASInfo[]
  /** Text for the copy control in the table corner; omitted when there is nothing to copy. */
  copyText?: () => string
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
            <span className="text-[11px] font-medium tracking-wider text-muted-foreground">
              {t('result.communities').toLocaleUpperCase('en-US')}
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

interface BgpTableColumn {
  id: string
  colClass: string
  headClass: string
  cellClass: string
  header: ReactNode
  renderCell: (route: BGPRoute) => ReactNode
}

function buildBgpTableColumns(
  t: (key: string) => string,
  enriched: ASInfo[],
  options: {
    showNode: boolean
    showLocalPref: boolean
    showMed: boolean
    showCommunities: boolean
  },
): BgpTableColumn[] {
  const columns: BgpTableColumn[] = [
    {
      id: 'star',
      colClass: 'result-bgp-table__col-star',
      headClass: 'result-bgp-table__star',
      cellClass: 'result-bgp-table__star text-center align-middle',
      header: <span className="sr-only">{t('result.best')}</span>,
      renderCell: route => route.best ? (
        <Star
          className="w-3.5 h-3.5 mx-auto fill-amber-400 text-amber-400"
          aria-label={t('result.best_route')}
        />
      ) : null,
    },
  ]

  if (options.showNode) {
    columns.push({
      id: 'node',
      colClass: 'result-bgp-table__col-node',
      headClass: 'result-bgp-table__node',
      cellClass: 'result-bgp-table__node text-xs font-medium text-muted-foreground whitespace-nowrap',
      header: t('result.node'),
      renderCell: route => route.node_name || '—',
    })
  }

  columns.push(
    {
      id: 'age',
      colClass: 'result-bgp-table__col-age',
      headClass: 'result-bgp-table__age',
      cellClass: 'result-bgp-table__age font-data text-xs tabular-nums text-muted-foreground whitespace-nowrap',
      header: t('result.age'),
      renderCell: route => route.age || '—',
    },
    {
      id: 'prefix',
      colClass: 'result-bgp-table__col-prefix',
      headClass: 'result-bgp-table__prefix',
      cellClass: 'result-bgp-table__prefix font-data text-sm font-medium whitespace-nowrap',
      header: t('result.prefix'),
      renderCell: route => route.prefix || '—',
    },
    {
      id: 'as-path',
      colClass: 'result-bgp-table__col-as-path',
      headClass: 'result-bgp-table__as-path',
      cellClass: 'result-bgp-table__as-path relative overflow-visible font-data text-xs text-muted-foreground',
      header: t('result.as_path'),
      renderCell: route => (
        <AsPathInline path={route.as_path} enriched={enriched} tooltipPlacement="bottom" nowrap />
      ),
    },
  )

  if (options.showLocalPref) {
    columns.push({
      id: 'local-pref',
      colClass: 'result-bgp-table__col-metric',
      headClass: 'result-bgp-table__metric',
      cellClass: 'result-bgp-table__metric font-data text-xs tabular-nums text-muted-foreground whitespace-nowrap',
      header: <span className="sr-only">{t('result.local_pref')}</span>,
      renderCell: route => formatMetric(route.local_pref),
    })
  }

  if (options.showMed) {
    columns.push({
      id: 'med',
      colClass: 'result-bgp-table__col-metric',
      headClass: 'result-bgp-table__metric',
      cellClass: 'result-bgp-table__metric font-data text-xs tabular-nums text-muted-foreground whitespace-nowrap',
      header: t('result.med'),
      renderCell: route => formatMetric(route.med),
    })
  }

  if (options.showCommunities) {
    columns.push({
      id: 'communities',
      colClass: 'result-bgp-table__col-communities',
      headClass: 'result-bgp-table__communities',
      cellClass: 'result-bgp-table__communities',
      header: t('result.communities'),
      renderCell: route => (
        <div className="result-bgp-table__communities-inner min-w-0">
          <RouteMetaBlock route={route} compact />
        </div>
      ),
    })
  }

  return columns
}

export function ResultBGP({ result, enriched, copyText }: Props) {
  const { copied, copy } = useCopyFeedback(copyText ?? (() => ''))
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

  const tableColumns = useMemo(
    () => buildBgpTableColumns(t, enriched, {
      showNode: uniqueNodeNames.size > 1,
      showLocalPref: shouldShowRouteMetric(routes, 'local_pref'),
      showMed: shouldShowRouteMetric(routes, 'med'),
      showCommunities: routes.some(routeHasMeta),
    }),
    [t, enriched, routes, uniqueNodeNames],
  )

  if (routes.length === 0) return null

  const showNode = uniqueNodeNames.size > 1
  const showLocalPref = shouldShowRouteMetric(routes, 'local_pref')
  const showMed = shouldShowRouteMetric(routes, 'med')
  const showMobileAsPath = routes.length > 1

  // Clicking the result copies it, but not when the click was finishing a selection —
  // people highlight prefixes and AS paths out of this table.
  const copyOnClick = copyText ? () => { if (!isTextSelectionClick()) copy() } : undefined

  return (
    <div className="result-bgp-copyable">
      <div
        className="result-table-frame animate-fade-up md:hidden divide-y divide-border"
        onClick={copyOnClick}
      >
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

      <div className="result-table-frame hidden animate-fade-up md:block" onClick={copyOnClick}>
        <Table className="result-bgp-table md:table" containerClassName="result-table-wrap">
          <colgroup>
            {tableColumns.map(column => (
              <col key={column.id} className={column.colClass} />
            ))}
          </colgroup>
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              {tableColumns.map(column => (
                <TableHead key={column.id} className={`${column.headClass} text-center`}>
                  {column.header}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {routes.map((route, i) => (
              <TableRow key={`${route.prefix}-${i}`} className="relative hover:z-20">
                {tableColumns.map(column => (
                  <TableCell key={column.id} className={column.cellClass}>
                    {column.renderCell(route)}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {copyText && (
        <div className="result-bgp-copyable__actions">
          <CopyResultButton copied={copied} onCopy={copy} />
        </div>
      )}
    </div>
  )
}
