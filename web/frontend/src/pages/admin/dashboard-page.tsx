import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Server, FileText, Globe, Network, Cpu } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/ui/page-header'
import { AdminPanel } from '@/components/admin/admin-panel'
import { ResourceUsageCell } from '@/components/admin/resource-usage-cell'
import { VersionStatCard } from '@/components/admin/version-stat-card'
import { api } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import { formatMemoryDetail, formatCpuDetail, resourceLevelLabel } from '@/lib/resource-level'
import type { Node, AuditEntry, GeoIPStatus, BGPNeighbor, SystemStatus } from '@/types/domain'
import { commandBadgeLabel, formatAuditParams } from '@/lib/audit-params'

function formatGeoDate(iso: string, locale: string, style: 'short' | 'medium' = 'medium'): string {
  if (!iso) return '—'
  try {
    return new Intl.DateTimeFormat(locale, { dateStyle: style, timeStyle: 'short' }).format(new Date(iso))
  } catch {
    return iso
  }
}

function BGPStatCard({ neighbors }: { neighbors: BGPNeighbor[] }) {
  const { t } = useI18n()

  if (neighbors.length === 0) return null

  const established = neighbors.filter(n => n.status === 'established')
  const connected = established.length
  const totalRoutes = established.reduce((sum, n) => sum + (n.prefixes_received ?? 0), 0)
  const iconClass = connected === neighbors.length
    ? 'text-success-on-surface'
    : connected > 0
      ? 'text-warning-on-surface'
      : 'text-muted-foreground'

  return (
    <Link to="/admin/bgp-neighbors" className="block h-full">
      <div className="admin-stat-card">
        <div className={`admin-stat-card__icon ${iconClass}`}>
          <Network className="w-[1.125rem] h-[1.125rem]" />
        </div>
        <div className="min-w-0">
          <div className="admin-stat-card__label">{t('admin.bgp_sessions')}</div>
          <div className="admin-stat-card__value font-data">{connected}/{neighbors.length}</div>
          <div className="admin-stat-card__meta space-y-0.5">
            <div>
              {t('admin.bgp_sessions_hint')
                .replace('{connected}', String(connected))
                .replace('{total}', String(neighbors.length))}
            </div>
            {connected > 0 && (
              <div className="font-data tabular-nums">
                {t('admin.bgp_total_routes').replace('{routes}', totalRoutes.toLocaleString())}
              </div>
            )}
          </div>
        </div>
      </div>
    </Link>
  )
}

function GeoIPStatCard({ status }: { status: GeoIPStatus | null }) {
  const { t, locale } = useI18n()

  if (!status?.configured) return null

  const lastDL = status.last_download
  const iconClass = status.enabled ? 'text-success-on-surface' : 'text-warning-on-surface'

  return (
    <Link to="/admin/geoip" className="block h-full">
      <div className="admin-stat-card">
        <div className={`admin-stat-card__icon ${iconClass}`}>
          <Globe className="w-[1.125rem] h-[1.125rem]" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="admin-stat-card__label">{t('admin.maxmind')}</div>
          <div className="admin-stat-card__value truncate">
            {status.enabled ? t('admin.maxmind_active') : t('admin.maxmind_inactive')}
          </div>
          <div className="admin-stat-card__meta space-y-0.5">
            <div className="truncate">
              {t('admin.maxmind_last_download')}: {lastDL ? formatGeoDate(lastDL, locale) : t('admin.maxmind_never')}
            </div>
            <div>
              ASN {status.asn_loaded ? '✓' : '·'} · City {status.city_loaded ? '✓' : '·'}
              {status.update_interval && <span className="ml-1">· {status.update_interval}</span>}
            </div>
          </div>
        </div>
      </div>
    </Link>
  )
}

function SystemResourcesPanel({ status, loading }: { status: SystemStatus | null; loading: boolean }) {
  const { t } = useI18n()

  const rows = status
    ? [
        {
          key: 'cpu',
          label: t('admin.resource_cpu'),
          resource: status.cpu,
          available: status.cpu_available,
          detail: formatCpuDetail(status, t),
        },
        {
          key: 'memory',
          label: t('admin.resource_memory'),
          resource: status.memory,
          available: true,
          detail: formatMemoryDetail(status),
        },
      ]
    : []

  return (
    <AdminPanel padded={false}>
      <div className="px-4 pt-4 pb-3 sm:px-5 flex flex-wrap items-end justify-between gap-2">
        <div>
          <h2 className="admin-dashboard-recent__title">{t('admin.system_resources')}</h2>
          <p className="text-xs text-muted-foreground mt-1">{t('admin.system_thresholds_hint')}</p>
        </div>
        <div className="admin-stat-card__icon text-brand-accent mb-0.5">
          <Cpu className="w-[1.125rem] h-[1.125rem]" />
        </div>
      </div>
      {loading && !status ? (
        <div className="px-4 pb-4 sm:px-5 space-y-2">
          <Skeleton className="h-10 w-full rounded-lg" />
          <Skeleton className="h-10 w-full rounded-lg" />
        </div>
      ) : rows.length === 0 ? (
        <p className="px-4 pb-4 sm:px-5 text-muted-foreground text-xs">{t('admin.resource_unavailable')}</p>
      ) : (
        <div className="admin-dashboard-recent-wrap">
          <div className="admin-dashboard-recent admin-dashboard-recent--resources">
            <div className="admin-dashboard-recent__head">
              <span>{t('admin.system_resource')}</span>
              <span>{t('admin.system_usage')}</span>
              <span>{t('admin.system_detail')}</span>
              <span>{t('admin.status')}</span>
            </div>
            {rows.map(row => (
              <div key={row.key} className="admin-dashboard-recent__row">
                <span className="font-medium text-sm">{row.label}</span>
                <span>
                  <ResourceUsageCell
                    resource={row.resource}
                    available={row.available}
                    level={row.resource.level}
                  />
                </span>
                <span className="admin-dashboard-recent__detail admin-dashboard-recent__detail--resource font-data text-muted-foreground">
                  {row.detail}
                </span>
                <span className="admin-dashboard-recent__status">
                  <Badge
                    variant={row.available ? (row.resource.level === 'critical' ? 'destructive' : row.resource.level === 'warning' ? 'warning' : 'success') : 'secondary'}
                    className="text-[10px] px-1.5 py-0"
                  >
                    {row.available ? resourceLevelLabel(row.resource.level, t) : t('admin.resource_unavailable')}
                  </Badge>
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </AdminPanel>
  )
}

export function DashboardPage() {
  const { t } = useI18n()
  const [stats, setStats] = useState<{ nodesActive: number; nodesTotal: number; queriesTotal: number; queriesToday: number } | null>(null)
  const [recent, setRecent] = useState<AuditEntry[]>([])
  const [geoipStatus, setGeoipStatus] = useState<GeoIPStatus | null>(null)
  const [bgpNeighbors, setBgpNeighbors] = useState<BGPNeighbor[]>([])
  const [systemStatus, setSystemStatus] = useState<SystemStatus | null>(null)
  const [systemLoading, setSystemLoading] = useState(true)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    async function loadSystemStatus(showLoading: boolean) {
      if (showLoading) setSystemLoading(true)
      try {
        const data = await api.get<SystemStatus>('/admin/system/status')
        if (!cancelled) setSystemStatus(data)
      } catch {
        if (!cancelled) setSystemStatus(null)
      } finally {
        if (!cancelled) setSystemLoading(false)
      }
    }
    loadSystemStatus(true)
    const timer = window.setInterval(() => loadSystemStatus(false), 15000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [])

  useEffect(() => {
    Promise.all([
      api.get<Node[]>('/admin/nodes'),
      fetch('/api/v1/admin/audit?limit=10', { credentials: 'include' }).then(res => res.json()),
      api.get<GeoIPStatus>('/admin/geoip/status').catch(() => null),
      api.get<BGPNeighbor[]>('/admin/bgp-neighbors').catch(() => []),
    ]).then(([nodes, audit, geoip, neighbors]) => {
      const auditData: AuditEntry[] = audit.data ?? []
      const nodesActive = nodes.filter(node => node.active).length
      setStats({
        nodesActive,
        nodesTotal: nodes.length,
        queriesTotal: audit.meta?.total ?? 0,
        queriesToday: audit.meta?.today ?? 0,
      })
      setRecent(auditData)
      setGeoipStatus(geoip)
      setBgpNeighbors(neighbors ?? [])
    }).catch(() => {}).finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="admin-page space-y-6">
        <Skeleton className="h-16 w-full rounded-xl" />
        <div className="admin-stat-grid">
          <Skeleton className="h-[4.75rem] w-full rounded-xl" />
          <Skeleton className="h-[4.75rem] w-full rounded-xl" />
          <Skeleton className="h-[4.75rem] w-full rounded-xl" />
        </div>
        <Skeleton className="h-64 w-full rounded-xl" />
      </div>
    )
  }

  return (
    <div className="admin-page space-y-6">
      <PageHeader title={t('admin.dashboard')} eyebrow={t('admin.title')} />

      <div className="admin-stat-grid">
        {Boolean(geoipStatus?.configured) && <GeoIPStatCard status={geoipStatus} />}
        {bgpNeighbors.length > 0 && <BGPStatCard neighbors={bgpNeighbors} />}

        <Link to="/admin/audit" className="block h-full">
          <div className="admin-stat-card">
            <div className="admin-stat-card__icon text-brand-accent">
              <FileText className="w-[1.125rem] h-[1.125rem]" />
            </div>
            <div className="min-w-0">
              <div className="admin-stat-card__label">{t('admin.queries')}</div>
              <div className="admin-stat-card__value font-data">
                {stats?.queriesToday ?? 0}/{stats?.queriesTotal ?? 0}
              </div>
              <div className="admin-stat-card__meta">
                {t('admin.queries_hint')
                  .replace('{today}', String(stats?.queriesToday ?? 0))
                  .replace('{total}', String(stats?.queriesTotal ?? 0))}
              </div>
            </div>
          </div>
        </Link>

        <Link to="/admin/nodes" className="block h-full">
          <div className="admin-stat-card">
            <div className="admin-stat-card__icon text-brand-accent">
              <Server className="w-[1.125rem] h-[1.125rem]" />
            </div>
            <div className="min-w-0">
              <div className="admin-stat-card__label">{t('admin.nodes')}</div>
              <div className="admin-stat-card__value font-data">
                {stats?.nodesActive ?? 0}/{stats?.nodesTotal ?? 0}
              </div>
            </div>
          </div>
        </Link>

        <VersionStatCard />
      </div>

      <SystemResourcesPanel status={systemStatus} loading={systemLoading} />

      <AdminPanel padded={false}>
        <div className="px-4 pt-4 pb-3 sm:px-5">
          <h2 className="admin-dashboard-recent__title">{t('admin.recent_queries')}</h2>
        </div>
        {recent.length === 0 ? (
          <p className="px-4 pb-4 sm:px-5 text-muted-foreground text-xs">{t('admin.no_queries')}</p>
        ) : (
          <div className="admin-dashboard-recent-wrap">
            <div className="admin-dashboard-recent">
              <div className="admin-dashboard-recent__head">
                <span>{t('admin.time')}</span>
                <span>{t('query.node')}</span>
                <span>{t('admin.query')}</span>
                <span>{t('admin.type')}</span>
                <span>{t('admin.source_ip')}</span>
                <span>{t('admin.duration')}</span>
                <span>{t('admin.status')}</span>
              </div>
              {recent.map((e: AuditEntry) => (
                <div key={e.id} className="admin-dashboard-recent__row">
                  <span className="admin-dashboard-recent__time">
                    {e.created_at
                      ? new Date(e.created_at.replace(' ', 'T')).toLocaleString(undefined, {
                          month: 'short',
                          day: 'numeric',
                          hour: '2-digit',
                          minute: '2-digit',
                        })
                      : '—'}
                  </span>
                  <span className="admin-dashboard-recent__node">{e.node_name || '—'}</span>
                  <span className="admin-dashboard-recent__params" title={formatAuditParams(e.params)}>
                    {formatAuditParams(e.params)}
                  </span>
                  <span className="admin-dashboard-recent__type">
                    <Badge variant="info" className="normal-case tracking-normal text-[10px] px-1.5 py-0">
                      {commandBadgeLabel(e.command, t)}
                    </Badge>
                  </span>
                  <span className="admin-dashboard-recent__ip">{e.source_ip || '—'}</span>
                  <span className="admin-dashboard-recent__duration">
                    {e.duration_ms > 0 ? `${e.duration_ms}ms` : '—'}
                  </span>
                  <span className="admin-dashboard-recent__status">
                    <Badge variant={e.success ? 'success' : 'destructive'} className="text-[10px] px-1.5 py-0">
                      {e.success ? t('admin.ok') : t('admin.error')}
                    </Badge>
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </AdminPanel>
    </div>
  )
}
