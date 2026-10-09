import { useEffect, useState } from 'react'
import { ChevronLeft, ChevronRight, Download } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { PageHeader } from '@/components/ui/page-header'
import { AdminPanel } from '@/components/admin/admin-panel'
import { ClickableIP, GeoIPLookupDialog } from '@/components/admin/geoip-lookup-dialog'
import { exportAuditCSV } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import type { AuditEntry } from '@/types/domain'
import { auditPageWindow } from '@/lib/audit-pages'
import { commandBadgeLabel, extractAuditTarget } from '@/lib/audit-params'

export function AuditPage() {
  const { t } = useI18n()
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const [geoipIp, setGeoipIp] = useState<string | null>(null)
  const limit = 20

  useEffect(() => {
    fetch(`/api/v1/admin/audit?limit=${limit}&page=${page - 1}`, {
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
    })
      .then(res => {
        if (res.status === 401) { window.location.href = '/admin/login'; throw new Error('Unauthorized') }
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        return res.json()
      })
      .then(json => {
        setEntries(json.data ?? [])
        setTotal(json.meta?.total ?? 0)
      })
      .catch(() => {})
  }, [page])

  const totalPages = Math.ceil(total / limit)

  return (
    <div className="admin-page space-y-6">
      <GeoIPLookupDialog
        ip={geoipIp ?? ''}
        open={geoipIp !== null}
        onOpenChange={open => { if (!open) setGeoipIp(null) }}
      />
      <PageHeader title={t('admin.audit')}>
        <Button variant="outline" onClick={exportAuditCSV}><Download className="w-4 h-4 mr-1" /> {t('admin.export_csv')}</Button>
      </PageHeader>
      <AdminPanel padded={false}>
        <div className="admin-table-wrap">
      <Table containerClassName="admin-table-inner">
        <TableHeader>
          <TableRow>
            <TableHead>{t('admin.time')}</TableHead>
            <TableHead>{t('admin.source_ip')}</TableHead>
            <TableHead>{t('admin.query')}</TableHead>
            <TableHead>{t('query.node')}</TableHead>
            <TableHead>{t('admin.type')}</TableHead>
            <TableHead>{t('admin.duration')}</TableHead>
            <TableHead>{t('admin.status')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map(e => (
            <TableRow key={e.id}>
              <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{e.created_at ? new Date(e.created_at.replace(' ', 'T')).toLocaleString() : '-'}</TableCell>
              <TableCell className="text-xs whitespace-nowrap">
                <ClickableIP value={e.source_ip} onSelect={setGeoipIp} className="text-xs" />
              </TableCell>
              <TableCell className="text-sm max-w-xs truncate">
                <ClickableIP value={extractAuditTarget(e.params)} onSelect={setGeoipIp} className="text-sm" />
              </TableCell>
              <TableCell className="text-sm">{e.node_name || '—'}</TableCell>
              <TableCell>
                <Badge variant="info" className="normal-case tracking-normal">{commandBadgeLabel(e.command, t)}</Badge>
              </TableCell>
              <TableCell className="text-sm whitespace-nowrap">{e.duration_ms}ms</TableCell>
              <TableCell><Badge variant={e.success ? 'success' : 'destructive'}>{e.success ? t('admin.ok') : t('admin.error')}</Badge></TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
        </div>
      </AdminPanel>
      {totalPages > 1 && (
        <div className="flex flex-wrap items-center justify-center gap-1">
          <Button
            variant="outline"
            size="sm"
            className="px-2"
            aria-label={t('admin.audit_prev')}
            disabled={page <= 1}
            onClick={() => setPage(current => Math.max(1, current - 1))}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          {auditPageWindow(page, totalPages).map((item, index) => (
            item === 'gap' ? (
              <span key={`gap-${index}`} className="px-1 text-sm text-muted-foreground" aria-hidden>…</span>
            ) : (
              <Button key={item} variant={page === item ? 'default' : 'outline'} size="sm" onClick={() => setPage(item)}>{item}</Button>
            )
          ))}
          <Button
            variant="outline"
            size="sm"
            className="px-2"
            aria-label={t('admin.audit_next')}
            disabled={page >= totalPages}
            onClick={() => setPage(current => Math.min(totalPages, current + 1))}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      )}
    </div>
  )
}
