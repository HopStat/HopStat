import { useState } from 'react'
import { Check, Link2, Server } from 'lucide-react'
import { useI18n } from '@/contexts/i18n-context'

interface Props {
  command: string
  target: string
  nodeName?: string
  shareUrl: string
}

const buttonClass =
  'flex items-center gap-1.5 px-2 py-1 rounded-md text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors'

export function ResultShareBar({ command, target, nodeName, shareUrl }: Props) {
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
    <div className="result-share-bar">
      <span className="result-share-bar__command">{command}</span>
      <span className="result-share-bar__target">{target}</span>
      {nodeName && (
        <span className="result-share-bar__node">
          <Server className="w-3 h-3" aria-hidden />
          {nodeName}
        </span>
      )}
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
