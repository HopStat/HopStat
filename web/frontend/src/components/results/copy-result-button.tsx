import { Check, Copy } from 'lucide-react'
import { useI18n } from '@/contexts/i18n-context'

interface Props {
  copied: boolean
  onCopy: () => void
  className?: string
}

export function CopyResultButton({ copied, onCopy, className }: Props) {
  const { t } = useI18n()

  return (
    <button
      type="button"
      onClick={e => { e.stopPropagation(); onCopy() }}
      aria-label={t('result.copy_result')}
      className={`flex items-center gap-1 rounded-md px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground ${className ?? ''}`}
    >
      {copied ? (
        <><Check className="h-3 w-3 text-brand" aria-hidden /><span>{t('result.copied')}</span></>
      ) : (
        <><Copy className="h-3 w-3" aria-hidden /><span>{t('result.copy_result')}</span></>
      )}
    </button>
  )
}
