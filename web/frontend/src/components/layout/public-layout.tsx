import { useEffect, Fragment } from 'react'
import { Outlet, Link } from 'react-router-dom'
import { Search, Sun, Moon, Globe2, Network, Mail, FileText, Shield } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/contexts/theme-context'
import { useI18n } from '@/contexts/i18n-context'
import { useSettings } from '@/contexts/settings-context'

export function PublicLayout() {
  const { theme, toggleTheme } = useTheme()
  const { t } = useI18n()
  const { settings, loading } = useSettings()

  useEffect(() => {
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    if (link && settings.logo_path) {
      link.href = settings.logo_path
    }
  }, [settings.logo_path])

  const siteName = settings.site_name || 'Looking Glass'
  const siteDesc = settings.site_description || ''
  const headerBg = settings.header_color || '#1e293b'

  const footerLinks = [
    { url: settings.url_website, icon: Globe2, label: t('footer.website') },
    { url: settings.url_contact, icon: Mail, label: t('footer.contact') },
    { url: settings.url_terms, icon: FileText, label: t('footer.terms') },
    { url: settings.url_privacy, icon: Shield, label: t('footer.privacy') },
    { url: settings.url_peeringdb, icon: Network, label: t('footer.peeringdb') },
  ].filter(l => l.url)

  return (
    <div className="min-h-screen flex flex-col">
      <header className="text-white shadow-lg" style={{ backgroundColor: headerBg }}>
        <div className="max-w-7xl mx-auto px-4 sm:px-6 h-16 flex items-center justify-between">
          <Link to="/" className="flex items-center gap-2.5">
            {!loading && settings.logo_path ? (
              <img src={settings.logo_path} alt={siteName} className="h-10 w-10 object-contain" />
            ) : (
              <Search className="w-5 h-5" />
            )}
            <div className="flex flex-col">
              <span className="font-bold text-lg leading-tight">{siteName}</span>
              {siteDesc && <span className="text-[11px] text-white/80 leading-tight">{siteDesc}</span>}
            </div>
          </Link>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="icon" className="text-white hover:bg-white/10" onClick={toggleTheme}>
              {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
            </Button>
          </div>
        </div>
      </header>
      <main className="flex-1">
        <Outlet />
      </main>
      <footer className="text-white" style={{ backgroundColor: headerBg }}>
        <div className="max-w-7xl mx-auto px-4 sm:px-6 py-4 flex items-center justify-center gap-2 flex-wrap">
          {footerLinks.map((l, i) => (
            <Fragment key={l.label}>
              {i > 0 && <span className="text-white/20 mx-1 select-none">|</span>}
              <a href={l.url} target="_blank" rel="noopener noreferrer"
                className="text-white/60 hover:text-white transition-colors inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-md hover:bg-white/10 text-sm" title={l.label}>
                <l.icon className="w-3.5 h-3.5" />
                <span className="text-xs">{l.label}</span>
              </a>
            </Fragment>
          ))}
          {footerLinks.length > 0 && <span className="text-white/20 mx-1 select-none">|</span>}
          <a
            href="https://github.com/HopStat/HopStat"
            target="_blank"
            rel="noopener noreferrer"
            className="text-white/60 hover:text-white transition-colors inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-md hover:bg-white/10"
          >
            <svg viewBox="0 0 16 16" className="w-3.5 h-3.5 fill-current" aria-hidden="true">
              <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
            </svg>
            <span className="text-xs">HopStat</span>
          </a>
        </div>
      </footer>
    </div>
  )
}
