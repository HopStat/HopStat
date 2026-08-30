import { useState } from 'react'
import { Check, Link2 } from 'lucide-react'
import { useI18n } from '@/contexts/i18n-context'

interface Props {
  summary: string
  shareUrl: string
}

const buttonClass =
  'flex items-center gap-1 px-2 py-1 rounded-md text-[11px] text-muted-foreground hover:text-foreground hover:bg-accent transition-colors'

export function ResultShareBar({ summary, shareUrl }: Props) {
  const { t } = useI18n()
  const [copied, setCopied] = useState<'link' | null>(null)

  async function copy(kind: 'link', text: string) {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      return
    }
    setCopied(kind)
    setTimeout(() => setCopied(current => (current === kind ? null : current)), 2000)
  }

  return (
    <div className="result-share-bar flex min-w-0 items-center gap-2 px-1">
      <span className="truncate font-data text-[11px] text-muted-foreground">{summary}</span>
      <div className="ml-auto flex shrink-0 items-center gap-1">
        <button
          type="button"
          onClick={() => void copy('link', shareUrl)}
          className={buttonClass}
          title={shareUrl}
        >
          {copied === 'link' ? (
            <><Check className="w-3 h-3 text-brand-accent" aria-hidden /><span>{t('result.link_copied')}</span></>
          ) : (
            <><Link2 className="w-3 h-3" aria-hidden /><span>{t('result.copy_link')}</span></>
          )}
        </button>
      </div>
    </div>
  )
}
