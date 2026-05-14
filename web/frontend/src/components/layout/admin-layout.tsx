import { Outlet, Link } from 'react-router-dom'
import { Sun, Moon, LogOut, Menu, X } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/contexts/theme-context'
import { useI18n } from '@/contexts/i18n-context'
import { useAuth } from '@/contexts/auth-context'
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

export function AdminLayout() {
  const { theme, toggleTheme } = useTheme()
  const { t } = useI18n()
  const { logout } = useAuth()
  const [sidebarOpen, setSidebarOpen] = useState(false)

  return (
    <div className="min-h-screen flex">
      {sidebarOpen && <div className="fixed inset-0 z-30 bg-black/50 lg:hidden" onClick={() => setSidebarOpen(false)} />}

      <aside className={cn(
        'fixed inset-y-0 left-0 z-40 w-64 bg-sidebar border-r border-sidebar-border transform transition-transform lg:translate-x-0 lg:static lg:inset-0',
        sidebarOpen ? 'translate-x-0' : '-translate-x-full'
      )}>
        <div className="flex items-center justify-between h-16 px-4 border-b border-sidebar-border">
          <Link to="/admin" className="font-bold text-lg text-sidebar-foreground">{t('admin.title')}</Link>
          <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setSidebarOpen(false)}><X className="w-5 h-5" /></Button>
        </div>
        <nav className="p-4 space-y-1">
          {navItems.map((item, i) => (
            <Link key={item.path} to={item.path} onClick={() => setSidebarOpen(false)}
              className="flex items-center gap-3 px-3 py-2 rounded-md text-sm text-sidebar-foreground hover:bg-accent transition-colors">
              <span>{item.icon}</span>
              <span>{t(item.labelKey)}</span>
            </Link>
          ))}
        </nav>
      </aside>

      <div className="flex-1 flex flex-col min-w-0">
        <header className="h-16 border-b bg-background flex items-center justify-between px-4 sm:px-6">
          <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setSidebarOpen(true)}><Menu className="w-5 h-5" /></Button>
          <div className="flex-1" />
          <div className="flex items-center gap-2">
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
