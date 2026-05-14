import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useI18n } from '@/contexts/i18n-context'
import type { ASPathResult } from '@/types/domain'

interface Props { result: ASPathResult }

export function ResultASPath({ result }: Props) {
  const { t } = useI18n()
  return (
    <div>
      <h3 className="text-lg font-bold mb-3">AS{result.asn}</h3>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('result.prefix')}</TableHead>
            <TableHead>{t('result.as_path')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(result.prefixes ?? []).map((entry, i) => (
            <TableRow key={i}>
              <TableCell className="font-mono">{entry.prefix}</TableCell>
              <TableCell>
                <div className="flex items-center gap-1 flex-wrap">
                  {(entry.as_path ?? []).map((asn, j) => <Badge key={j} variant="info" className="text-xs">AS{asn}</Badge>)}
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
