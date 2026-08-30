import { useEffect, useRef, useState } from 'react'
import { ArrowRight, RefreshCw } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api-client'
import { useI18n } from '@/contexts/i18n-context'
import type { UpdateStatus } from '@/types/domain'
import { waitForHealth } from '@/lib/wait-for-health'
import { getLastSeenVersion, setLastSeenVersion } from '@/lib/version-seen'
import { WhatsNewDialog } from '@/components/admin/whats-new-dialog'
import { formatReleaseVersion } from '@/lib/release-notes-format'

type UpdateState = 'idle' | 'applying' | 'restarting'

export function VersionStatCard() {
  const { t } = useI18n()
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [updateError, setUpdateError] = useState<string | null>(null)
  const [updateState, setUpdateState] = useState<UpdateState>('idle')
  const [whatsNewOpen, setWhatsNewOpen] = useState(false)
  const [whatsNewVersion, setWhatsNewVersion] = useState('')
  const [whatsNewFromVersion, setWhatsNewFromVersion] = useState('')
  const [whatsNewReleaseVersions, setWhatsNewReleaseVersions] = useState<string[] | undefined>()
  const [whatsNewReleaseNotes, setWhatsNewReleaseNotes] = useState<string | undefined>()
  const restartAbortRef = useRef<AbortController | null>(null)

  function loadStatus() {
    return api
      .get<UpdateStatus>('/admin/update/status')
      .then(data => {
        setStatus(data)
        setLoadError(null)
        if (!getLastSeenVersion()) {
          setLastSeenVersion(data.current)
        }
      })
      .catch((err: Error) => setLoadError(err.message))
  }

  async function loadReleaseNotes(since: string) {
    const sinceParam = since.trim()
    const path = sinceParam
      ? `/admin/update/status?since=${encodeURIComponent(sinceParam)}`
      : '/admin/update/status'
    const data = await api.get<UpdateStatus>(path)
    setWhatsNewReleaseVersions(data.release_versions)
    setWhatsNewReleaseNotes(data.release_notes)
    return data
  }

  useEffect(() => {
    void loadStatus()
  }, [])

  useEffect(() => {
    return () => {
      restartAbortRef.current?.abort()
    }
  }, [])

  async function openWhatsNew(version: string, since?: string) {
    const sinceVersion = since?.trim() || getLastSeenVersion() || status?.current || ''
    try {
      await loadReleaseNotes(sinceVersion)
    } catch {
      setWhatsNewReleaseNotes(status?.release_notes)
    }
    setWhatsNewFromVersion(status?.current || '')
    setWhatsNewVersion(version)
    setWhatsNewOpen(true)
  }

  function dismissWhatsNew() {
    if (!status?.update_available) {
      const version = whatsNewVersion || status?.current || ''
      if (version) {
        setLastSeenVersion(version)
      }
    }
  }

  async function applyUpdate() {
    if (!status?.update_available || !status.self_update_enabled || updateState !== 'idle') return
    const targetVersion = status.latest
    setUpdateState('applying')
    setUpdateError(null)
    try {
      await api.post('/admin/update/apply')
      setUpdateState('restarting')
      setLastSeenVersion(targetVersion)
      restartAbortRef.current?.abort()
      restartAbortRef.current = new AbortController()
      await waitForHealth({
        expectedVersion: targetVersion,
        signal: restartAbortRef.current.signal,
      })
      window.location.reload()
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') return
      setUpdateError(t('admin.update_failed'))
      setUpdateState('idle')
    }
  }

  function handleCardClick() {
    if (!status || updateState === 'applying' || updateState === 'restarting') return
    const target = status.update_available ? status.latest : status.current
    void openWhatsNew(target, getLastSeenVersion() || status.current)
  }

  function handleCardKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    if (event.key !== 'Enter' && event.key !== ' ') return
    event.preventDefault()
    handleCardClick()
  }

  function handleRetryClick(event: React.MouseEvent<HTMLButtonElement>) {
    event.stopPropagation()
    void loadStatus()
  }

  if (loadError) {
    return (
      <div className="admin-version-card admin-version-card--error">
        <div className="admin-version-card__header">
          <span className="admin-version-card__label">{t('admin.version')}</span>
          <Badge variant="destructive" className="text-[10px] px-1.5 py-0">
            {t('admin.version_status_error')}
          </Badge>
        </div>
        <p className="admin-version-card__hint text-destructive" title={loadError}>
          {loadError}
        </p>
        <div className="admin-version-card__actions">
          <Button size="sm" variant="outline" className="h-7 text-xs" onClick={handleRetryClick}>
            <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
            {t('admin.retry')}
          </Button>
        </div>
      </div>
    )
  }

  if (!status) {
    return <Skeleton className="admin-version-card h-[5rem] w-full rounded-xl" />
  }

  const isUpdating = updateState === 'applying' || updateState === 'restarting'
  const current = formatReleaseVersion(status.current)
  const latest = formatReleaseVersion(status.latest)
  const cardState = status.update_available ? 'update' : 'current'

  return (
    <>
      <div
        className={`admin-version-card admin-version-card--interactive admin-version-card--${cardState}${isUpdating ? ' admin-version-card--busy' : ''}`}
        role="button"
        tabIndex={isUpdating ? -1 : 0}
        aria-disabled={isUpdating}
        aria-label={t('admin.whats_new_open')}
        onClick={handleCardClick}
        onKeyDown={handleCardKeyDown}
      >
        <div className="admin-version-card__header">
          <span className="admin-version-card__label">{t('admin.version')}</span>
          <Badge variant={status.update_available ? 'warning' : 'success'} className="text-[10px] px-1.5 py-0">
            {status.update_available ? t('admin.update_available') : t('admin.up_to_date')}
          </Badge>
        </div>

        <div className="admin-version-card__path" aria-hidden={isUpdating}>
          {status.update_available ? (
            <>
              <span className="admin-version-card__ver">{current}</span>
              <ArrowRight className="admin-version-card__arrow" aria-hidden />
              <span className="admin-version-card__ver admin-version-card__ver--target">{latest}</span>
            </>
          ) : (
            <span className="admin-version-card__ver admin-version-card__ver--solo">{current}</span>
          )}
        </div>

        <p className="admin-version-card__hint">
          {isUpdating
            ? t('admin.update_applying')
            : status.update_available
              ? t('admin.version_update_ready')
              : t('admin.version_latest_installed')}
        </p>

        {updateError ? (
          <p className="admin-version-card__hint text-destructive">{updateError}</p>
        ) : null}
      </div>

      <WhatsNewDialog
        open={whatsNewOpen}
        onOpenChange={setWhatsNewOpen}
        currentVersion={whatsNewFromVersion || status.current}
        version={whatsNewVersion || status.latest}
        releaseVersions={whatsNewReleaseVersions ?? status.release_versions}
        releaseNotes={whatsNewReleaseNotes ?? status.release_notes}
        releaseUrl={status.release_url}
        updateAvailable={status.update_available}
        selfUpdateEnabled={status.self_update_enabled}
        onUpdate={applyUpdate}
        confirming={isUpdating}
        onDismiss={dismissWhatsNew}
      />
    </>
  )
}
