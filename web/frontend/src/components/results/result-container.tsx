import { useQueryStream } from '@/hooks/use-sse'
import { useSettings } from '@/contexts/settings-context'
import { useI18n } from '@/contexts/i18n-context'
import { OutputTerminal } from './output-terminal'

interface Props {
  queryId: string | null
}

export function ResultContainer({ queryId }: Props) {
  const { settings } = useSettings()
  const { t } = useI18n()
  const { result, lines, error } = useQueryStream(queryId)
  const headerColor = settings.header_color || '#1e293b'

  if (!queryId) return null

  if (error && lines.length === 0) {
    return <div className="mt-4 p-4 rounded-md bg-destructive/10 text-destructive text-sm">{error}</div>
  }

  const isRunning = !result || result.status === 'pending' || result.status === 'running'
  const isError = result?.status === 'error'

  return (
    <div className="mt-4 space-y-4">
      {lines.length > 0 && (
        <OutputTerminal lines={lines} isRunning={isRunning} accentColor={headerColor} />
      )}

      {lines.length === 0 && isRunning && (
        <div className="flex items-center justify-center py-8 text-muted-foreground">
          <span className="w-2 h-2 rounded-full bg-muted-foreground/50 animate-pulse mr-2" />
          <span className="text-sm">{t('result.waiting')}</span>
        </div>
      )}

      {isError && result && (
        <div className="p-4 rounded-md bg-destructive/10 text-destructive text-sm">{result.error_msg || t('query.failed')}</div>
      )}
    </div>
  )
}
