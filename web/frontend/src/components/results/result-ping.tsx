import { Card, CardContent } from '@/components/ui/card'
import { formatRTT, formatLoss } from '@/lib/utils'
import { useI18n } from '@/contexts/i18n-context'
import type { PingResult } from '@/types/domain'

interface Props { result: PingResult }

export function ResultPing({ result }: Props) {
  const { t } = useI18n()
  const lossColor = result.packet_loss < 5 ? 'text-green-600 dark:text-green-400' : result.packet_loss < 25 ? 'text-yellow-600 dark:text-yellow-400' : 'text-red-600 dark:text-red-400'
  const stats = [
    { label: t('result.sent'), value: result.packets_sent },
    { label: t('result.received'), value: result.packets_recv },
    { label: t('result.loss'), value: formatLoss(result.packet_loss), className: lossColor },
    { label: t('result.min_rtt'), value: formatRTT(result.min_rtt) },
    { label: t('result.avg_rtt'), value: formatRTT(result.avg_rtt) },
    { label: t('result.max_rtt'), value: formatRTT(result.max_rtt) },
  ]

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
      {stats.map(s => (
        <Card key={s.label}>
          <CardContent className="p-4 text-center">
            <div className="text-xs text-muted-foreground">{s.label}</div>
            <div className={`text-lg font-bold ${s.className || ''}`}>{s.value}</div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
