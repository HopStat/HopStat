import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatRTT, formatLoss } from '@/lib/utils'
import { useI18n } from '@/contexts/i18n-context'
import type { MTRResult } from '@/types/domain'

interface Props { result: MTRResult }

export function ResultMTR({ result }: Props) {
  const { t } = useI18n()
  return (
    <div className="overflow-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-12">#</TableHead>
            <TableHead>{t('result.host')}</TableHead>
            <TableHead>{t('result.loss')}</TableHead>
            <TableHead>{t('result.sent')}</TableHead>
            <TableHead>{t('result.recv')}</TableHead>
            <TableHead>{t('result.avg')}</TableHead>
            <TableHead>{t('result.best')}</TableHead>
            <TableHead>{t('result.worst')}</TableHead>
            <TableHead>{t('result.as')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(result.hops ?? []).map(hop => (
            <TableRow key={hop.number}>
              <TableCell className="font-mono text-muted-foreground">{hop.number}</TableCell>
              <TableCell>{hop.host || '???'}</TableCell>
              <TableCell className="font-mono text-sm">{formatLoss(hop.loss)}</TableCell>
              <TableCell className="font-mono text-sm">{hop.sent}</TableCell>
              <TableCell className="font-mono text-sm">{hop.recv}</TableCell>
              <TableCell className="font-mono text-sm">{formatRTT(hop.avg)}</TableCell>
              <TableCell className="font-mono text-sm">{formatRTT(hop.best)}</TableCell>
              <TableCell className="font-mono text-sm">{formatRTT(hop.worst)}</TableCell>
              <TableCell>{hop.as_info && <Badge variant="info">AS{hop.as_info.asn}</Badge>}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
