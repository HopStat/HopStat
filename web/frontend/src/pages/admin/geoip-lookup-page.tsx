import { useState } from 'react'
import { Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PageHeader } from '@/components/ui/page-header'
import { CountryFlag } from '@/components/ui/country-flag'
import { api } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import type { ASInfo, GeoIPLookupReport, GeoIPResolveSource } from '@/types/domain'

function formatAS(info: ASInfo | undefined): string {
  if (!info?.asn) return '—'
  const org = info.org_name || info.short_name
  return org ? `AS${info.asn} — ${org}` : `AS${info.asn}`
}

function sourceBadgeVariant(source: GeoIPResolveSource): 'default' | 'secondary' | 'outline' {
  if (source === 'blocks' || source === 'mmdb') return 'default'
  if (source === 'dns') return 'secondary'
  return 'outline'
}

function SourceRow({
  label,
  candidate,
  chosen,
  t,
}: {
  label: string
  candidate: GeoIPLookupReport['blocks']
  chosen: boolean
  t: (key: string) => string
}) {
  let status = t('admin.geoip_lookup_unavailable')
  if (!candidate.available) {
    status = t('admin.geoip_lookup_not_loaded')
  } else if (candidate.error) {
    status = candidate.error
  } else if (candidate.matched) {
    status = formatAS(candidate.info)
  } else {
    status = t('admin.geoip_lookup_no_match')
  }

  return (
    <TableRow className={chosen ? 'bg-brand/5' : undefined}>
      <TableCell className="font-medium whitespace-nowrap">
        <div className="inline-flex items-center justify-center gap-2">
          <span>{label}</span>
          {chosen && (
            <Badge variant="default" className="text-[10px] uppercase tracking-wide">
              {t('admin.geoip_lookup_used')}
            </Badge>
          )}
        </div>
      </TableCell>
      <TableCell className="font-data text-sm">{status}</TableCell>
      <TableCell className="font-data text-xs text-muted-foreground whitespace-nowrap">
        {candidate.network || '—'}
      </TableCell>
    </TableRow>
  )
}

export function GeoIPLookupPage() {
  const { t } = useI18n()
  const [ip, setIp] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [report, setReport] = useState<GeoIPLookupReport | null>(null)

  async function handleLookup(e?: React.FormEvent) {
    e?.preventDefault()
    const query = ip.trim()
    if (!query) return
    setLoading(true)
    setError(null)
    try {
      const data = await api.get<GeoIPLookupReport>(`/admin/geoip/lookup?ip=${encodeURIComponent(query)}`)
      setReport(data)
    } catch (err) {
      setReport(null)
      setError(err instanceof Error ? err.message : t('admin.geoip_lookup_failed'))
    } finally {
      setLoading(false)
    }
  }

  const sourceLabel = (source: GeoIPResolveSource) => {
    switch (source) {
      case 'blocks': return t('admin.geoip_source_blocks')
      case 'mmdb': return t('admin.geoip_source_mmdb')
      case 'dns': return t('admin.geoip_source_dns')
      default: return t('admin.geoip_source_none')
    }
  }

  return (
    <div className="admin-page space-y-6">
      <PageHeader title={t('admin.geoip_lookup')} description={t('admin.geoip_lookup_desc')} eyebrow={t('admin.title')} />

      <Card>
        <CardHeader>
          <CardTitle>{t('admin.geoip_lookup_query')}</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleLookup} className="flex flex-col sm:flex-row gap-3">
            <div className="flex-1 space-y-2">
              <Label htmlFor="geoip-ip">{t('admin.geoip_lookup_ip')}</Label>
              <Input
                id="geoip-ip"
                value={ip}
                onChange={e => setIp(e.target.value)}
                placeholder="8.8.8.8"
                className="font-data"
                autoComplete="off"
              />
            </div>
            <div className="flex items-end">
              <Button type="submit" disabled={loading || !ip.trim()}>
                <Search className="w-4 h-4" />
                {loading ? t('admin.geoip_lookup_searching') : t('admin.geoip_lookup_search')}
              </Button>
            </div>
          </form>
          {error && <p className="mt-3 text-sm text-destructive">{error}</p>}
        </CardContent>
      </Card>

      {report && (
        <>
          <Card>
            <CardHeader className="flex flex-row items-center justify-between gap-3 space-y-0">
              <CardTitle>{t('admin.geoip_lookup_result')}</CardTitle>
              <Badge variant={sourceBadgeVariant(report.chosen_source)}>
                {sourceLabel(report.chosen_source)}
              </Badge>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="rounded-lg border border-border/80 bg-muted/20 px-4 py-3 font-data text-sm">
                {formatAS(report.result)}
                {report.result?.country_code && (
                  <span className="inline-flex items-center gap-1.5 ml-3 text-muted-foreground">
                    <CountryFlag code={report.result.country_code} />
                    {report.result.country_code}
                  </span>
                )}
              </div>

              {report.city && (
                <div className="text-sm text-muted-foreground">
                  {t('admin.geoip_lookup_city')}: {[report.city.city, report.city.country].filter(Boolean).join(', ') || '—'}
                </div>
              )}

              <p className="text-xs text-muted-foreground">{report.cache_note}</p>
              <p className="text-xs text-muted-foreground">
                {t('admin.geoip_lookup_blocks_csv')}: {report.blocks_csv_loaded ? report.blocks_csv_count.toLocaleString() : t('admin.geoip_lookup_not_loaded')}
                {report.asn_index_enriched && (
                  <span className="ml-2">· {t('admin.geoip_lookup_index_enriched')}</span>
                )}
              </p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t('admin.geoip_lookup_sources')}</CardTitle>
            </CardHeader>
            <CardContent className="p-0 overflow-x-auto">
              <div className="admin-table-wrap">
              <Table containerClassName="admin-table-inner">
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('admin.geoip_lookup_source')}</TableHead>
                    <TableHead>{t('admin.geoip_lookup_match')}</TableHead>
                    <TableHead>{t('admin.geoip_lookup_prefix')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <SourceRow
                    label={t('admin.geoip_source_blocks')}
                    candidate={report.blocks}
                    chosen={report.chosen_source === 'blocks'}
                    t={t}
                  />
                  <SourceRow
                    label={t('admin.geoip_source_mmdb')}
                    candidate={report.mmdb}
                    chosen={report.chosen_source === 'mmdb'}
                    t={t}
                  />
                  <SourceRow
                    label={t('admin.geoip_source_dns')}
                    candidate={report.dns}
                    chosen={report.chosen_source === 'dns'}
                    t={t}
                  />
                </TableBody>
              </Table>
              </div>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
