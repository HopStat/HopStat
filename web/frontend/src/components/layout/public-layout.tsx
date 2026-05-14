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
        <div className="max-w-7xl mx-auto px-4 sm:px-6 py-4 flex items-center justify-center gap-2">
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
        </div>
      </footer>
    </div>
  )
}
