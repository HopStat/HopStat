import { ExternalLink } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ReleaseNotesContent } from '@/lib/release-notes'
import { useI18n } from '@/contexts/i18n-context'

export type WhatsNewMode = 'confirm' | 'view' | 'post_update'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  version: string
  releaseName?: string
  releaseNotes?: string
  releaseUrl?: string
  mode: WhatsNewMode
  onConfirm?: () => void
  confirming?: boolean
  onDismiss?: () => void
}

function formatVersion(v: string): string {
  return v.startsWith('v') ? v : `v${v}`
}

export function WhatsNewDialog({
  open,
  onOpenChange,
  version,
  releaseName,
  releaseNotes,
  releaseUrl,
  mode,
  onConfirm,
  confirming = false,
  onDismiss,
}: Props) {
  const { t } = useI18n()
  const titleVersion = formatVersion(version)
  const notes = releaseNotes?.trim() ?? ''

  function handleOpenChange(next: boolean) {
    if (!next && mode !== 'confirm') {
      onDismiss?.()
    }
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-lg gap-0 overflow-hidden p-0 sm:max-w-xl">
        <DialogHeader className="space-y-1 border-b border-border/60 px-6 py-5 text-left">
          <DialogTitle>{t('admin.whats_new_title').replace('{{version}}', titleVersion)}</DialogTitle>
          {releaseName ? (
            <DialogDescription className="text-left">{releaseName}</DialogDescription>
          ) : null}
        </DialogHeader>

        <div className="max-h-[min(50vh,24rem)] overflow-y-auto px-6 py-4">
          {notes ? (
            <ReleaseNotesContent markdown={notes} />
          ) : (
            <p lang="en" className="text-sm text-muted-foreground">
              {t('admin.whats_new_empty')}
            </p>
          )}
        </div>

        <DialogFooter className="flex-col gap-2 border-t border-border/60 px-6 py-4 sm:flex-row sm:justify-between">
          {releaseUrl ? (
            <a
              href={releaseUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-xs text-brand hover:underline"
            >
              {t('admin.whats_new_github')}
              <ExternalLink className="h-3 w-3" />
            </a>
          ) : (
            <span />
          )}
          <div className="flex w-full flex-col-reverse gap-2 sm:w-auto sm:flex-row">
            {mode === 'confirm' ? (
              <>
                <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={confirming}>
                  {t('admin.whats_new_cancel')}
                </Button>
                <Button type="button" onClick={onConfirm} disabled={confirming}>
                  {confirming ? t('admin.update_applying') : t('admin.update_apply')}
                </Button>
              </>
            ) : (
              <Button
                type="button"
                onClick={() => {
                  onDismiss?.()
                  onOpenChange(false)
                }}
              >
                {t('admin.whats_new_got_it')}
              </Button>
            )}
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
