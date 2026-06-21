import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Globe2, Network, Mail, FileText, Shield, Tags } from 'lucide-react'
import { useI18n } from '@/contexts/i18n-context'
import { useSettings } from '@/contexts/settings-context'
import { api } from '@/lib/api-client'
import { HopStatDocsLink, hopstatIconLinkClass } from '@/components/layout/hopstat-docs-link'
import type { CommunityRule } from '@/types/domain'

const footerIconLinkClass = hopstatIconLinkClass

export function SiteFooter() {
  const { t } = useI18n()
  const { settings } = useSettings()
  const [hasCommunities, setHasCommunities] = useState(false)

  useEffect(() => {
    api.get<CommunityRule[]>('/communities')
      .then(data => setHasCommunities(data.length > 0))
      .catch(() => setHasCommunities(false))
  }, [])

  const externalLinks = [
    { url: settings.url_website, icon: Globe2, label: t('footer.website') },
    { url: settings.url_contact, icon: Mail, label: t('footer.contact') },
    { url: settings.url_terms, icon: FileText, label: t('footer.terms') },
    { url: settings.url_privacy, icon: Shield, label: t('footer.privacy') },
    { url: settings.url_peeringdb, icon: Network, label: t('footer.peeringdb') },
  ].filter(l => l.url)

  const hasLeftLinks = hasCommunities || externalLinks.length > 0

  return (
    <footer className="fixed bottom-0 inset-x-0 z-40 border-t border-border bg-card pb-[env(safe-area-inset-bottom)]">
      <div className="max-w-5xl mx-auto px-4 sm:px-6 py-1 sm:py-2 flex items-center justify-between gap-2 sm:gap-2 min-h-[2.25rem] sm:min-h-[2.5rem]">
        <div className="flex items-center gap-1.5 sm:gap-1.5 flex-nowrap min-w-0 overflow-hidden">
          {hasCommunities && (
            <Link
              to="/communities"
              title={t('footer.communities')}
              className={footerIconLinkClass}
            >
              <Tags className="w-3 h-3 sm:w-3.5 sm:h-3.5 shrink-0" />
              <span className="hidden min-[420px]:inline whitespace-nowrap">{t('footer.communities')}</span>
            </Link>
          )}

          {externalLinks.map(l => (
            <a
              key={l.url}
              href={l.url}
              target="_blank"
              rel="noopener noreferrer"
              title={l.label}
              className={footerIconLinkClass}
            >
              <l.icon className="w-3 h-3 sm:w-3.5 sm:h-3.5 shrink-0" />
              <span className="hidden min-[420px]:inline whitespace-nowrap">{l.label}</span>
            </a>
          ))}

          {!hasLeftLinks && (
            <span aria-hidden className="shrink-0" />
          )}
        </div>

        <HopStatDocsLink className="ml-auto" />
      </div>
    </footer>
  )
}
