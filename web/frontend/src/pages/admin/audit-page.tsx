import { useEffect, useState } from 'react'
import { Download } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { exportAuditCSV } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import type { AuditEntry } from '@/types/domain'

function getToken(): string | null {
  return localStorage.getItem('jwt_token')
}

export function AuditPage() {
  const { t } = useI18n()
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const limit = 20

  useEffect(() => {
    const token = getToken()
    fetch(`/api/v1/admin/audit?limit=${limit}&page=${page - 1}`, {
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    })
      .then(res => res.json())
      .then(json => {
        setEntries(json.data ?? [])
        setTotal(json.meta?.total ?? 0)
      })
      .catch(() => {})
  }, [page])

  const totalPages = Math.ceil(total / limit)

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t('admin.audit')}</h1>
        <Button variant="outline" onClick={exportAuditCSV}><Download className="w-4 h-4 mr-1" /> {t('admin.export_csv')}</Button>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('admin.time')}</TableHead>
            <TableHead>{t('admin.source_ip')}</TableHead>
            <TableHead>{t('query.command')}</TableHead>
            <TableHead className="hidden md:table-cell">{t('admin.params')}</TableHead>
            <TableHead>{t('admin.duration')}</TableHead>
            <TableHead>{t('admin.status')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map(e => (
            <TableRow key={e.id}>
              <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{e.created_at ? new Date(e.created_at.replace(' ', 'T')).toLocaleString() : '-'}</TableCell>
              <TableCell className="font-mono text-sm">{e.source_ip}</TableCell>
              <TableCell><Badge variant="info">{e.command}</Badge></TableCell>
              <TableCell className="hidden md:table-cell font-mono text-xs max-w-xs truncate">{e.params}</TableCell>
              <TableCell className="text-sm">{e.duration_ms}ms</TableCell>
              <TableCell><Badge variant={e.success ? 'success' : 'destructive'}>{e.success ? t('admin.ok') : t('admin.error')}</Badge></TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {totalPages > 1 && (
        <div className="flex justify-center gap-1">
          {Array.from({ length: Math.min(totalPages, 10) }, (_, i) => (
            <Button key={i + 1} variant={page === i + 1 ? 'default' : 'outline'} size="sm" onClick={() => setPage(i + 1)}>{i + 1}</Button>
          ))}
        </div>
      )}
    </div>
  )
}
