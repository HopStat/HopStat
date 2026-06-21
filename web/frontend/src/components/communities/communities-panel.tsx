import { useEffect, useMemo, useState } from 'react'
import { X } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { QueryErrorAlert } from '@/components/results/query-error-alert'
import { api } from '@/lib/api-client'
import { communityRowAccent, communitySeverityBadgeVariant, communitySeverityLabel } from '@/lib/community-severity'
import { useI18n } from '@/contexts/i18n-context'
import type { CommunityRule } from '@/types/domain'

interface Props {
  onClose: () => void
}

function sortCommunities(rules: CommunityRule[]): CommunityRule[] {
  return [...rules].sort((a, b) =>
    a.community.localeCompare(b.community, undefined, { numeric: true, sensitivity: 'base' }),
  )
}

export function CommunitiesPanel({ onClose }: Props) {
  const { t } = useI18n()
  const [rules, setRules] = useState<CommunityRule[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    setLoading(true)
    setError('')
    api.get<CommunityRule[]>('/communities')
      .then(data => setRules(sortCommunities(data)))
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : t('communities.load_failed'))
      })
      .finally(() => setLoading(false))
  }, [t])

  const sortedRules = useMemo(() => sortCommunities(rules), [rules])

  return (
    <div className="communities-panel">
      <div className="communities-panel__toolbar">
        <button
          type="button"
          onClick={onClose}
          title={t('communities.back')}
          className="inline-flex items-center justify-center rounded-md border border-border bg-background p-1.5 text-muted-foreground hover:text-foreground hover:bg-accent transition-colors shrink-0"
        >
          <X className="w-4 h-4" />
        </button>
      </div>

      {error && <QueryErrorAlert message={error} />}

      {loading && (
        <div className="communities-table-frame communities-table-frame--state flex items-center justify-center py-10">
          <span className="w-2 h-2 rounded-full bg-brand animate-pulse mr-2" />
          <span className="text-sm text-muted-foreground font-medium">{t('communities.loading')}</span>
        </div>
      )}

      {!loading && !error && sortedRules.length === 0 && (
        <div className="communities-table-frame communities-table-frame--state">
          <p className="text-sm text-muted-foreground py-8 text-center">{t('communities.empty')}</p>
        </div>
      )}

      {!loading && sortedRules.length > 0 && (
        <div className="communities-table-frame">
          <Table containerClassName="communities-table-wrap">
            <TableHeader>
              <TableRow>
                <TableHead className="w-[11rem]">{t('communities.community')}</TableHead>
                <TableHead>{t('communities.description')}</TableHead>
                <TableHead className="w-[7rem] hidden sm:table-cell">{t('communities.severity')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedRules.map(rule => {
                const accent = communityRowAccent(rule.severity)
                const message = rule.message_i18n?.trim()
                return (
                  <TableRow key={rule.id} className={`border-l-4 ${accent}`}>
                    <TableCell className="align-top py-3.5">
                      <Badge
                        variant={communitySeverityBadgeVariant(rule.severity)}
                        className="font-mono text-xs normal-case tracking-normal px-2.5 py-1"
                      >
                        {rule.community}
                      </Badge>
                    </TableCell>
                    <TableCell className="align-top py-3.5 text-sm text-foreground/90 leading-relaxed">
                      {message || <span className="text-muted-foreground">—</span>}
                    </TableCell>
                    <TableCell className="align-top py-3.5 hidden sm:table-cell">
                      <Badge variant={communitySeverityBadgeVariant(rule.severity)}>
                        {communitySeverityLabel(rule.severity, t)}
                      </Badge>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
