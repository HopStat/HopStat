import { useEffect } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { useSettings } from '@/contexts/settings-context'
import { useI18n } from '@/contexts/i18n-context'
import { SiteFooter } from '@/components/layout/site-footer'
import { SiteDocumentHead } from '@/components/layout/site-document-head'

export function PublicLayout() {
  const { settings } = useSettings()
  const { t } = useI18n()
  const location = useLocation()
  const isHome = location.pathname === '/'
  const isCommunities = location.pathname === '/communities'

  useEffect(() => {
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    if (link && settings.logo_path) {
      link.href = settings.logo_path
    }
  }, [settings.logo_path])

  return (
    <div className="min-h-screen corporate-background pb-[calc(2.75rem+env(safe-area-inset-bottom))] sm:pb-[calc(3.5rem+env(safe-area-inset-bottom))]">
      <SiteDocumentHead
        structuredData={isHome}
        pageTitle={isCommunities ? t('communities.title') : undefined}
      />
      <main className="relative">
        <Outlet />
      </main>
      <SiteFooter />
    </div>
  )
}
