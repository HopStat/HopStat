import { Outlet, Link } from 'react-router-dom'
import { Sun, Moon, LogOut, Menu, X } from 'lucide-react'
import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/contexts/theme-context'
import { useI18n } from '@/contexts/i18n-context'
import { useAuth } from '@/contexts/auth-context'
import { api } from '@/lib/api-client'
import { cn } from '@/lib/utils'

const navItems = [
  { path: '/admin', labelKey: 'admin.dashboard', icon: '📊', end: true },
  { path: '/admin/nodes', labelKey: 'admin.nodes', icon: '🖥️' },
  { path: '/admin/audit', labelKey: 'admin.audit', icon: '📋' },
  { path: '/admin/users', labelKey: 'admin.users', icon: '👥' },
  { path: '/admin/community-rules', labelKey: 'admin.community_rules', icon: '🛡️' },
  { path: '/admin/bgp-neighbors', labelKey: 'admin.bgp_neighbors', icon: '🌐' },
  { path: '/admin/settings', labelKey: 'admin.settings', icon: '⚙️' },
]

interface UpdateStatus {
  current: string
  latest: string
  update_available: boolean
  release_url: string
}

function VersionBadge() {
  const [status, setStatus] = useState<UpdateStatus | null>(null)

  useEffect(() => {
    api.get<UpdateStatus>('/admin/update/status')
      .then(setStatus)
      .catch(() => {})
  }, [])

  if (!status) return null

  const color = status.update_available
    ? 'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300 border-amber-300 dark:border-amber-700'
    : 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300 border-green-300 dark:border-green-700'

  const label = status.update_available
    ? `v${status.current} → v${status.latest}`
    : `v${status.current}`

  const inner = (
    <span className={cn('text-[11px] font-mono px-2 py-0.5 rounded-full border', color)}>
      {label}
    </span>
  )

  return status.update_available ? (
    <a href={status.release_url} target="_blank" rel="noopener noreferrer" title="Update available" className="hover:opacity-80 transition-opacity">
      {inner}
    </a>
  ) : inner
}

export function AdminLayout() {
  const { theme, toggleTheme } = useTheme()
  const { t } = useI18n()
  const { logout } = useAuth()
  const [sidebarOpen, setSidebarOpen] = useState(false)

  return (
    <div className="min-h-screen flex">
      {sidebarOpen && <div className="fixed inset-0 z-30 bg-black/50 lg:hidden" onClick={() => setSidebarOpen(false)} />}

      <aside className={cn(
        'fixed inset-y-0 left-0 z-40 w-64 bg-sidebar border-r border-sidebar-border transform transition-transform lg:translate-x-0 lg:static lg:inset-0 flex flex-col',
        sidebarOpen ? 'translate-x-0' : '-translate-x-full'
      )}>
        <div className="flex items-center justify-between h-16 px-4 border-b border-sidebar-border shrink-0">
          <Link to="/admin" className="font-bold text-lg text-sidebar-foreground">{t('admin.title')}</Link>
          <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setSidebarOpen(false)}><X className="w-5 h-5" /></Button>
        </div>
        <nav className="p-4 space-y-1 flex-1">
          {navItems.map((item) => (
            <Link key={item.path} to={item.path} onClick={() => setSidebarOpen(false)}
              className="flex items-center gap-3 px-3 py-2 rounded-md text-sm text-sidebar-foreground hover:bg-accent transition-colors">
              <span>{item.icon}</span>
              <span>{t(item.labelKey)}</span>
            </Link>
          ))}
        </nav>
        <div className="px-4 py-3 border-t border-sidebar-border shrink-0">
          <a
            href="https://github.com/HopStat/HopStat"
            target="_blank"
            rel="noopener noreferrer"
            className="flex items-center gap-2 text-xs text-sidebar-foreground/50 hover:text-sidebar-foreground/80 transition-colors"
          >
            <svg viewBox="0 0 16 16" className="w-3.5 h-3.5 fill-current" aria-hidden="true">
              <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
            </svg>
            HopStat/HopStat
          </a>
        </div>
      </aside>

      <div className="flex-1 flex flex-col min-w-0">
        <header className="h-16 border-b bg-background flex items-center justify-between px-4 sm:px-6">
          <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setSidebarOpen(true)}><Menu className="w-5 h-5" /></Button>
          <div className="flex-1" />
          <div className="flex items-center gap-3">
            <VersionBadge />
            <Button variant="ghost" size="icon" onClick={toggleTheme}>
              {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
            </Button>
            <Button variant="ghost" size="sm" onClick={logout}><LogOut className="w-4 h-4 mr-1" />{t('nav.logout')}</Button>
          </div>
        </header>
        <main className="flex-1 p-4 sm:p-6 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
