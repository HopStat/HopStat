import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatRTT } from '@/lib/utils'
import { useI18n } from '@/contexts/i18n-context'
import type { TracerouteResult } from '@/types/domain'

interface Props { result: TracerouteResult }

export function ResultTraceroute({ result }: Props) {
  const { t } = useI18n()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-12">#</TableHead>
          <TableHead>{t('result.host')}</TableHead>
          <TableHead>{t('result.ip')}</TableHead>
          <TableHead>{t('result.rtt')}</TableHead>
          <TableHead>{t('result.as')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {(result.hops ?? []).map(hop => (
          <TableRow key={hop.number}>
            <TableCell className="font-mono text-muted-foreground">{hop.number}</TableCell>
            <TableCell>{hop.host || '*'}</TableCell>
            <TableCell className="font-mono text-sm">{hop.ip || '*'}</TableCell>
            <TableCell className="font-mono text-sm">{(hop.rtt ?? []).length ? (hop.rtt ?? []).map(r => formatRTT(r)).join(', ') : '*'}</TableCell>
            <TableCell>{hop.as_info && <Badge variant="info">AS{hop.as_info.asn}</Badge>}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
