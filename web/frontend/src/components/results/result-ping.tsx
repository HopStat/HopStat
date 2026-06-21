import { formatRTT, formatLoss } from '@/lib/utils'
import { useI18n } from '@/contexts/i18n-context'
import type { PingResult } from '@/types/domain'

interface Props {
  result: PingResult
  loading?: boolean
  running?: boolean
}

function displayValue(value: number | undefined, pending: boolean): string | number {
  if (value != null && Number.isFinite(value)) return value
  return pending ? '…' : '—'
}

function formatPingRtt(result: PingResult, ms: number | undefined, pending: boolean): string {
  if ((result.packets_recv ?? 0) <= 0) return pending ? '…' : '—'
  return formatRTT(ms)
}

export function ResultPing({ result, loading = false, running = false }: Props) {
  const { t } = useI18n()
  const loss = Number.isFinite(result.packet_loss) ? (result.packet_loss as number) : null
  const lossColor = loss == null
    ? ''
    : loss < 5
      ? 'text-green-600 dark:text-green-400'
      : loss < 25
        ? 'text-yellow-600 dark:text-yellow-400'
        : 'text-red-600 dark:text-red-400'
  const hasRecv = (result.packets_recv ?? 0) > 0
  const stats = [
    {
      label: t('result.sent'),
      value: displayValue(result.packets_sent, running && result.packets_sent == null),
    },
    {
      label: t('result.received'),
      value: displayValue(result.packets_recv, running && !hasRecv),
    },
    {
      label: t('result.loss'),
      value: running ? '…' : formatLoss(result.packet_loss),
      className: running ? '' : lossColor,
    },
    { label: t('result.min_rtt'), value: formatPingRtt(result, result.min_rtt, running) },
    { label: t('result.avg_rtt'), value: formatPingRtt(result, result.avg_rtt, running) },
    { label: t('result.max_rtt'), value: formatPingRtt(result, result.max_rtt, running) },
  ]

  return (
    <div className="ping-stat-grid">
      {stats.map(s => (
        <div key={s.label} className="ping-stat-card">
          <div className="ping-stat-card__body">
            <div className="ping-stat-card__label" title={s.label}>
              {s.label}
            </div>
            <div className={`ping-stat-card__value ${s.className || ''}`}>
              {s.value}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}
