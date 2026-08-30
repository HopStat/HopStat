import { ArrowRight, ExternalLink, RefreshCw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ReleaseNotesPanel } from '@/lib/release-notes'
import { formatReleaseVersion } from '@/lib/release-notes-format'
import { useI18n } from '@/contexts/i18n-context'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentVersion?: string
  version: string
  releaseVersions?: string[]
  releaseNotes?: string
  releaseUrl?: string
  updateAvailable?: boolean
  selfUpdateEnabled?: boolean
  onUpdate?: () => void
  confirming?: boolean
  onDismiss?: () => void
}

function formatVersion(v: string): string {
  return formatReleaseVersion(v)
}

export function WhatsNewDialog({
  open,
  onOpenChange,
  currentVersion,
  version,
  releaseVersions,
  releaseNotes,
  releaseUrl,
  updateAvailable = false,
  selfUpdateEnabled = false,
  onUpdate,
  confirming = false,
  onDismiss,
}: Props) {
  const { t } = useI18n()
  const fromVersion = formatVersion(currentVersion || '')
  const toVersion = formatVersion(version || '')
  const showVersionPath = Boolean(fromVersion && toVersion && fromVersion !== toVersion)
  const hasNotes = Boolean(releaseNotes?.trim())
  const showUpdateAction = updateAvailable && selfUpdateEnabled

  function handleOpenChange(next: boolean) {
    if (!next && !confirming) {
      onDismiss?.()
    }
    if (!confirming) {
      onOpenChange(next)
    }
  }

  function handleDismiss() {
    onDismiss?.()
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="flex max-h-[min(85vh,36rem)] max-w-lg flex-col gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogHeader className="shrink-0 space-y-1 border-b border-border/60 px-6 py-4 text-left">
          <DialogTitle className="text-base">{t('admin.whats_new_title')}</DialogTitle>
          {showVersionPath ? (
            <DialogDescription asChild>
              <div className="admin-version-card__path mt-0.5">
                <span className="admin-version-card__ver text-xs">{fromVersion}</span>
                <ArrowRight className="admin-version-card__arrow" aria-hidden />
                <span className="admin-version-card__ver admin-version-card__ver--target text-xs">{toVersion}</span>
              </div>
            </DialogDescription>
          ) : null}
          {(releaseVersions?.length ?? 0) > 1 ? (
            <p className="text-xs text-muted-foreground">
              {t('admin.whats_new_versions').replace('{{count}}', String(releaseVersions?.length ?? 0))}
            </p>
          ) : null}
        </DialogHeader>

        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-6 py-3">
          {hasNotes ? (
            <ReleaseNotesPanel markdown={releaseNotes ?? ''} releaseVersions={releaseVersions} />
          ) : (
            <p lang="en" className="text-sm text-muted-foreground">
              {t('admin.whats_new_empty')}
            </p>
          )}
        </div>

        <DialogFooter className="shrink-0 flex-col gap-2 border-t border-border/60 px-6 py-3 sm:flex-row sm:justify-between">
          {releaseUrl ? (
            <a
              href={releaseUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-xs text-brand-accent hover:underline"
            >
              {t('admin.whats_new_github')}
              <ExternalLink className="h-3 w-3" />
            </a>
          ) : (
            <span />
          )}
          <div className="flex w-full sm:w-auto sm:justify-end">
            {showUpdateAction ? (
              <Button type="button" className="w-full sm:min-w-[9rem]" onClick={onUpdate} disabled={confirming}>
                {confirming ? (
                  <>
                    <RefreshCw className="mr-1.5 h-4 w-4 animate-spin" />
                    {t('admin.update_applying')}
                  </>
                ) : (
                  t('admin.update_apply')
                )}
              </Button>
            ) : (
              <Button type="button" className="w-full sm:min-w-[9rem]" onClick={handleDismiss} disabled={confirming}>
                {t('admin.whats_new_got_it')}
              </Button>
            )}
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
