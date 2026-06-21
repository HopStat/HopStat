import { useEffect, useState } from 'react'
import { Activity, GitBranch, Route } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useI18n } from '@/contexts/i18n-context'
import type { QuickQuery } from '@/types/domain'

interface Props {
  onQuickStart: (command: string, target: string, nodeId?: number | null) => void
}

const iconByCommand: Record<string, LucideIcon> = {
  ping: Activity,
  traceroute: Route,
  bgp_route: GitBranch,
}

const descKeyByCommand: Record<string, string> = {
  ping: 'query.home_ping_hint',
  traceroute: 'query.home_traceroute_hint',
  bgp_route: 'query.home_bgp_hint',
}

const commandLabelKeys: Record<string, string> = {
  ping: 'cmd.ping',
  traceroute: 'cmd.traceroute',
  bgp_route: 'cmd.bgp_route',
}

export function QueryHomeEmpty({ onQuickStart }: Props) {
  const { t } = useI18n()
  const [items, setItems] = useState<QuickQuery[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetch('/api/v1/quick-queries')
      .then(res => res.json())
      .then(json => setItems(json.data ?? []))
      .catch(() => setItems([]))
      .finally(() => setLoading(false))
  }, [])

  if (loading || items.length === 0) {
    return null
  }

  return (
    <section className="query-home-empty" aria-label={t('query.home_title')}>
      <div className="query-home-empty__intro">
        <p className="query-home-empty__eyebrow">{t('query.network_diagnostic')}</p>
        <h2 className="query-home-empty__title">{t('query.home_title')}</h2>
        <p className="query-home-empty__lead">{t('query.home_lead')}</p>
      </div>

      <ul className="query-home-empty__grid">
        {items.map(item => {
          const Icon = iconByCommand[item.command] ?? Activity
          const descKey = descKeyByCommand[item.command]
          const titleKey = commandLabelKeys[item.command]
          return (
            <li key={item.id}>
              <button
                type="button"
                className="query-home-card"
                onClick={() => onQuickStart(item.command, item.target, item.node_id ?? null)}
              >
                <span className="query-home-card__icon" aria-hidden>
                  <Icon className="w-4 h-4" />
                </span>
                <span className="query-home-card__body">
                  <span className="query-home-card__title">{titleKey ? t(titleKey) : item.command}</span>
                  {descKey && <span className="query-home-card__desc">{t(descKey)}</span>}
                </span>
                <span className="query-home-card__target-wrap">
                  {item.name && (
                    <span className="query-home-card__target-label">{item.name}</span>
                  )}
                  <span className="query-home-card__target font-data">{item.target}</span>
                </span>
              </button>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
