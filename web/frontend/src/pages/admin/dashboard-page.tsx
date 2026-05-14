import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Server, FileText, Users, RefreshCw, CheckCircle, AlertCircle } from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import type { Node, User, AuditEntry, UpdateStatus } from '@/types/domain'

function getToken(): string | null {
  return localStorage.getItem('jwt_token')
}

type UpdateState = 'idle' | 'applying' | 'restarting'

function VersionCard() {
  const { t } = useI18n()
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [updateState, setUpdateState] = useState<UpdateState>('idle')

  useEffect(() => {
    api.get<UpdateStatus>('/admin/update/status')
      .then(setStatus)
      .catch((err: Error) => setError(err.message))
  }, [])

  async function handleUpdate() {
    if (!status?.update_available || updateState !== 'idle') return
    setUpdateState('applying')
    try {
      await api.post('/admin/update/apply')
      setUpdateState('restarting')
      // Give the binary time to replace itself and restart, then reload
      setTimeout(() => window.location.reload(), 6000)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('admin.update_failed'))
      setUpdateState('idle')
    }
  }

  if (error) {
    return (
      <Card>
        <CardContent className="p-6 flex items-center gap-3 text-muted-foreground">
          <AlertCircle className="w-5 h-5 shrink-0" />
          <span className="text-sm">{t('admin.update_disabled')}</span>
        </CardContent>
      </Card>
    )
  }

  if (!status) {
    return <Skeleton className="h-20 w-full" />
  }

  const isRestarting = updateState === 'restarting'
  const isApplying = updateState === 'applying'

  return (
    <Card>
      <CardContent className="p-6 flex items-center justify-between gap-4 flex-wrap">
        <div className="flex items-center gap-4">
          {status.update_available ? (
            <AlertCircle className="w-6 h-6 text-amber-500 shrink-0" />
          ) : (
            <CheckCircle className="w-6 h-6 text-green-500 shrink-0" />
          )}
          <div>
            <div className="text-sm font-medium">
              {status.update_available ? t('admin.update_available') : t('admin.up_to_date')}
            </div>
            <div className="flex items-center gap-3 mt-1">
              <span className="text-xs text-muted-foreground">
                {t('admin.current_version')}: <span className="font-mono">{status.current}</span>
              </span>
              {status.update_available && (
                <span className="text-xs text-muted-foreground">
                  {t('admin.latest_version')}: <span className="font-mono text-amber-600">{status.latest}</span>
                </span>
              )}
            </div>
          </div>
        </div>

        <div className="flex items-center gap-3">
          {status.update_available && status.release_url && !isRestarting && (
            <a
              href={status.release_url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-xs text-muted-foreground underline underline-offset-2 hover:text-foreground"
            >
              {t('admin.release_notes')}
            </a>
          )}
          {isRestarting ? (
            <Badge variant="secondary" className="gap-1.5">
              <RefreshCw className="w-3 h-3 animate-spin" />
              {t('admin.update_restarting')}
            </Badge>
          ) : status.update_available ? (
            <Button
              size="sm"
              onClick={handleUpdate}
              disabled={isApplying}
              className="gap-1.5"
            >
              {isApplying ? (
                <>
                  <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                  {t('admin.update_applying')}
                </>
              ) : (
                t('admin.update_apply')
              )}
            </Button>
          ) : (
            <Badge variant="secondary" className="text-green-600">
              {status.current}
            </Badge>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

export function DashboardPage() {
  const { t } = useI18n()
  const [stats, setStats] = useState<{ nodes: number; users: number; queries: number } | null>(null)
  const [recent, setRecent] = useState<AuditEntry[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const token = getToken()
    Promise.all([
      api.get<Node[]>('/admin/nodes'),
      api.get<User[]>('/admin/users'),
      fetch('/api/v1/admin/audit?limit=10', {
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      }).then(res => res.json()),
    ]).then(([nodes, users, audit]) => {
      const auditData: AuditEntry[] = audit.data ?? []
      setStats({ nodes: nodes.length, users: users.length, queries: audit.meta?.total ?? 0 })
      setRecent(auditData)
    }).catch(() => {}).finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="space-y-4"><Skeleton className="h-24 w-full" /><Skeleton className="h-64 w-full" /></div>

  const cards = [
    { label: t('admin.nodes'), value: stats?.nodes ?? 0, icon: Server, color: 'text-blue-600', href: '/admin/nodes' },
    { label: t('admin.queries'), value: stats?.queries ?? 0, icon: FileText, color: 'text-purple-600', href: '/admin/audit' },
    { label: t('admin.users'), value: stats?.users ?? 0, icon: Users, color: 'text-orange-600', href: '/admin/users' },
  ]

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{t('admin.dashboard')}</h1>

      <VersionCard />

      <div className="grid grid-cols-2 lg:grid-cols-3 gap-4">
        {cards.map(c => (
          <Link key={c.label} to={c.href}>
            <Card className="hover:shadow-md transition-shadow">
              <CardContent className="p-6 flex items-center gap-4">
                <c.icon className={`w-8 h-8 ${c.color}`} />
                <div>
                  <div className="text-sm text-muted-foreground">{c.label}</div>
                  <div className="text-2xl font-bold">{c.value}</div>
                </div>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>

      <Card>
        <CardContent className="p-6">
          <h2 className="text-lg font-semibold mb-4">{t('admin.recent_queries')}</h2>
          {recent.length === 0 ? <p className="text-muted-foreground text-sm">{t('admin.no_queries')}</p> : (
            <div className="space-y-2">
              {recent.map((e: AuditEntry) => (
                <div key={e.id} className="flex items-center justify-between py-2 border-b last:border-0">
                  <div className="flex items-center gap-3">
                    <Badge variant="info">{e.command}</Badge>
                    <span className="text-sm text-muted-foreground font-mono truncate max-w-xs">{e.params}</span>
                  </div>
                  <span className="text-xs text-muted-foreground">{e.source_ip}</span>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
