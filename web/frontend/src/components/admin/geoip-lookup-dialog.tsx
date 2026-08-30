import { useEffect, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { CountryFlag } from '@/components/ui/country-flag'
import { api } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import {
  formatAS,
  formatSourceStatus,
  geoipSourceLabel,
  looksLikeIP,
  sourceBadgeVariant,
} from '@/lib/geoip-display'
import type { GeoIPLookupReport } from '@/types/domain'
import { cn } from '@/lib/utils'

const reportCache = new Map<string, GeoIPLookupReport>()

interface GeoIPLookupDialogProps {
  ip: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function GeoIPLookupDialog({ ip, open, onOpenChange }: GeoIPLookupDialogProps) {
  const { t } = useI18n()
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [fetched, setFetched] = useState<GeoIPLookupReport | null>(null)

  // A cached report is available synchronously, so it is read during render instead of
  // being copied into state by an effect — copying it cost an extra render and showed the
  // previous address's report for that frame.
  const cached = open && ip ? reportCache.get(ip) ?? null : null
  const report = cached ?? fetched

  useEffect(() => {
    if (!open || !ip || reportCache.get(ip)) return

    let cancelled = false
    const lookup = async () => {
      setLoading(true)
      setError(null)
      setFetched(null)
      try {
        const data = await api.get<GeoIPLookupReport>(`/admin/geoip/lookup?ip=${encodeURIComponent(ip)}`)
        if (cancelled) return
        reportCache.set(ip, data)
        setFetched(data)
      } catch (err) {
        if (cancelled) return
        setFetched(null)
        setError(err instanceof Error ? err.message : t('admin.geoip_lookup_failed'))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void lookup()
    return () => { cancelled = true }
  }, [open, ip, t])

  const sources = report
    ? [
        { key: 'blocks' as const, label: t('admin.geoip_source_blocks'), candidate: report.blocks },
        { key: 'mmdb' as const, label: t('admin.geoip_source_mmdb'), candidate: report.mmdb },
        { key: 'dns' as const, label: t('admin.geoip_source_dns'), candidate: report.dns },
      ]
    : []

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[calc(100%-2rem)] max-w-2xl max-h-[85vh] overflow-y-auto overflow-x-hidden p-6">
        <DialogHeader>
          <DialogTitle className="font-data">{ip}</DialogTitle>
        </DialogHeader>

        {loading && (
          <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            {t('admin.geoip_lookup_searching')}
          </div>
        )}

        {error && !loading && (
          <p className="text-sm text-destructive">{error}</p>
        )}

        {report && !loading && (
          <div className="min-w-0 space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={sourceBadgeVariant(report.chosen_source)}>
                {geoipSourceLabel(report.chosen_source, t)}
              </Badge>
            </div>

            <div className="rounded-lg border border-border/80 bg-muted/20 px-4 py-3 font-data text-sm break-words">
              {formatAS(report.result)}
              {report.result?.country_code && (
                <span className="inline-flex items-center gap-1.5 ml-3 text-muted-foreground">
                  <CountryFlag code={report.result.country_code} />
                  {report.result.country_code}
                </span>
              )}
            </div>

            {report.city && (
              <p className="text-sm text-muted-foreground">
                {t('admin.geoip_lookup_city')}: {[report.city.city, report.city.country].filter(Boolean).join(', ') || '—'}
              </p>
            )}

            <div className="geoip-dialog-table">
              <Table containerClassName="geoip-dialog-table__inner">
                <TableHeader>
                  <TableRow>
                    <TableHead className="geoip-dialog-table__source">{t('admin.geoip_lookup_source')}</TableHead>
                    <TableHead className="geoip-dialog-table__match">{t('admin.geoip_lookup_match')}</TableHead>
                    <TableHead className="geoip-dialog-table__prefix">{t('admin.geoip_lookup_prefix')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sources.map(({ key, label, candidate }) => (
                    <TableRow key={key} className={report.chosen_source === key ? 'bg-brand/5' : undefined}>
                      <TableCell className="geoip-dialog-table__source font-medium align-top">
                        <div className="flex flex-wrap items-center gap-2">
                          <span>{label}</span>
                          {report.chosen_source === key && (
                            <Badge variant="default" className="text-[10px] uppercase tracking-wide">
                              {t('admin.geoip_lookup_used')}
                            </Badge>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="geoip-dialog-table__match font-data text-sm align-top break-words">
                        {formatSourceStatus(candidate, t)}
                      </TableCell>
                      <TableCell className="geoip-dialog-table__prefix font-data text-xs text-muted-foreground align-top whitespace-nowrap">
                        {candidate.network || '—'}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

interface ClickableIPProps {
  value: string | undefined | null
  onSelect: (ip: string) => void
  className?: string
}

export function ClickableIP({ value, onSelect, className }: ClickableIPProps) {
  const text = (value ?? '').trim() || '—'
  if (!looksLikeIP(text)) {
    return <span className={cn('font-data', className)}>{text}</span>
  }

  return (
    <button
      type="button"
      className={cn(
        'font-data text-brand-accent hover:underline cursor-pointer bg-transparent border-0 p-0',
        className,
      )}
      onClick={() => onSelect(text)}
      title={text}
    >
      {text}
    </button>
  )
}
