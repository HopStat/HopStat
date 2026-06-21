import { useEffect, useState } from 'react'
import { Activity } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { useI18n } from '@/contexts/i18n-context'
import { cn } from '@/lib/utils'

export const HOPSTAT_DOCS_URL = 'https://docs.hopstat.net'

export const hopstatIconLinkClass =
  'inline-flex items-center gap-1 rounded-md px-1.5 sm:px-2 py-0.5 sm:py-1 text-[10px] sm:text-[11px] font-medium text-muted-foreground hover:text-foreground hover:bg-accent transition-colors shrink-0'

function formatVersion(v: string): string {
  return v.startsWith('v') ? v : `v${v}`
}

interface Props {
  className?: string
  showLabel?: boolean
  plainText?: boolean
}

export function HopStatDocsLink({ className, showLabel = false, plainText = false }: Props) {
  const { t } = useI18n()
  const [version, setVersion] = useState('')
  const [versionLoading, setVersionLoading] = useState(true)

  useEffect(() => {
    fetch('/health')
      .then(r => (r.ok ? r.json() : null))
      .then(data => {
        if (data?.version && typeof data.version === 'string') {
          setVersion(data.version)
        }
      })
      .catch(() => {})
      .finally(() => setVersionLoading(false))
  }, [])

  if (plainText) {
    return (
      <a
        href={HOPSTAT_DOCS_URL}
        target="_blank"
        rel="noopener noreferrer"
        title={version ? `HopStat ${formatVersion(version)}` : t('footer.hopstat')}
        aria-busy={versionLoading}
        className={cn(
          'admin-sidebar__hopstat-text',
          versionLoading && 'animate-pulse opacity-60',
          className,
        )}
      >
        <span className="inline-flex items-center justify-center gap-1.5">
          <span>HopStat</span>
          {version && (
            <Badge variant="outline" className="admin-sidebar__hopstat-version">
              {formatVersion(version)}
            </Badge>
          )}
        </span>
      </a>
    )
  }

  return (
    <a
      href={HOPSTAT_DOCS_URL}
      target="_blank"
      rel="noopener noreferrer"
      title={version ? `HopStat ${formatVersion(version)}` : t('footer.hopstat')}
      aria-busy={versionLoading}
      className={cn(
        hopstatIconLinkClass,
        'border border-border',
        showLabel && 'h-8 px-2 text-[11px] sm:text-xs',
        versionLoading && 'pointer-events-none hover:bg-transparent hover:text-muted-foreground',
        !showLabel && !versionLoading && version && 'group overflow-hidden',
        className,
      )}
    >
      <Activity
        className={cn(
          'w-3 h-3 sm:w-3.5 sm:h-3.5 shrink-0',
          versionLoading && 'animate-pulse opacity-60',
        )}
      />
      {showLabel ? (
        <span className="inline-flex items-center gap-1 whitespace-nowrap">
          <span>HopStat</span>
          {version && (
            <span className="font-data text-muted-foreground/80">{formatVersion(version)}</span>
          )}
        </span>
      ) : !versionLoading && (
        <span className="inline-flex max-w-0 items-center gap-1 overflow-hidden whitespace-nowrap opacity-0 transition-all duration-200 sm:group-hover:max-w-[8.5rem] sm:group-hover:opacity-100">
          <span>HopStat</span>
          {version && (
            <span className="font-data text-muted-foreground/80">{formatVersion(version)}</span>
          )}
        </span>
      )}
    </a>
  )
}
