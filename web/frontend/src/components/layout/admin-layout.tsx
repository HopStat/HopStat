import { Outlet, Link, NavLink } from 'react-router-dom'
import {
  Sun, Moon, Menu, X,
  LayoutDashboard, Server, ScrollText, Shield, Network, Settings, Globe, Zap,
} from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/contexts/theme-context'
import { useI18n } from '@/contexts/i18n-context'
import { useAuth } from '@/contexts/auth-context'
import { useSettings } from '@/contexts/settings-context'
import { LocaleSwitcher } from '@/components/query/locale-switcher'
import { HopStatDocsLink } from '@/components/layout/hopstat-docs-link'
import { cn } from '@/lib/utils'

const navItems = [
  { path: '/admin', labelKey: 'admin.dashboard', icon: LayoutDashboard, end: true },
  { path: '/admin/nodes', labelKey: 'admin.nodes', icon: Server },
  { path: '/admin/audit', labelKey: 'admin.audit', icon: ScrollText },
  { path: '/admin/community-rules', labelKey: 'admin.community_rules', icon: Shield },
  { path: '/admin/quick-queries', labelKey: 'admin.quick_queries', icon: Zap },
  { path: '/admin/bgp-neighbors', labelKey: 'admin.bgp_neighbors', icon: Network },
  { path: '/admin/geoip', labelKey: 'admin.geoip_lookup', icon: Globe },
  { path: '/admin/settings', labelKey: 'admin.settings', icon: Settings },
]

export function AdminLayout() {
  const { theme, toggleTheme } = useTheme()
  const { t } = useI18n()
  const { logout } = useAuth()
  const { settings } = useSettings()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const hasLogo = Boolean(settings.logo_path?.trim())
  const siteName = settings.site_name || 'Looking Glass'

  return (
    <div className="admin-shell">
      <div className="admin-atmosphere" aria-hidden="true">
        <div className="admin-atmosphere__glow" />
        <div className="admin-atmosphere__grid" />
      </div>

      {sidebarOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/45 lg:hidden cursor-pointer"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      <div className="admin-layout">
        <aside className={cn('admin-sidebar', sidebarOpen && 'admin-sidebar--open')}>
          <div className="admin-sidebar__brand">
            <Link
              to="/admin"
              className="admin-sidebar__brand-link"
              onClick={() => setSidebarOpen(false)}
            >
              {hasLogo && (
                <img
                  key={settings.logo_path}
                  src={settings.logo_path}
                  alt={siteName}
                  className="admin-sidebar__logo-img"
                />
              )}
              <div className="admin-sidebar__brand-text">
                <span className="admin-sidebar__title">{siteName}</span>
                <span className="admin-sidebar__subtitle">{t('admin.title')}</span>
              </div>
            </Link>
            <Button
              variant="ghost"
              size="icon"
              className="admin-sidebar__brand-close lg:hidden rounded-md h-8 w-8"
              onClick={() => setSidebarOpen(false)}
            >
              <X className="w-4 h-4" />
            </Button>
          </div>

          <nav className="admin-sidebar__nav">
            <ul className="admin-sidebar__nav-list">
              {navItems.map(item => (
                <li key={item.path}>
                  <NavLink
                    to={item.path}
                    end={item.end}
                    onClick={() => setSidebarOpen(false)}
                    className={({ isActive }) => cn('admin-nav-link', isActive && 'admin-nav-link--active')}
                  >
                    <item.icon className="admin-nav-link__icon" />
                    <span>{t(item.labelKey)}</span>
                  </NavLink>
                </li>
              ))}
            </ul>
          </nav>

          <div className="admin-sidebar__footer">
            <div className="admin-sidebar__footer-links">
              <Link
                to="/"
                className="admin-nav-link"
                onClick={() => setSidebarOpen(false)}
              >
                <span>{t('nav.home')}</span>
              </Link>
              <button
                type="button"
                className="admin-nav-link"
                onClick={logout}
              >
                <span>{t('nav.logout')}</span>
              </button>
            </div>
            <HopStatDocsLink plainText />
          </div>
        </aside>

        <div className="admin-content">
          <header className="admin-topbar">
            <Button
              variant="ghost"
              size="icon"
              className="lg:hidden rounded-md h-8 w-8"
              onClick={() => setSidebarOpen(true)}
            >
              <Menu className="w-4 h-4" />
            </Button>
            <div className="flex-1" />
            <div className="flex items-center gap-2">
              <LocaleSwitcher variant="admin" />
              <Button variant="ghost" size="icon" className="rounded-md h-8 w-8" onClick={toggleTheme}>
                {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
              </Button>
            </div>
          </header>

          <main className="admin-main">
            <div className="admin-main__inner">
              <Outlet />
            </div>
          </main>
        </div>
      </div>
    </div>
  )
}
