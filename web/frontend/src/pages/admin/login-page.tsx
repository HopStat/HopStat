import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Lock, Network, Sun, Moon } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { SiteDocumentHead } from '@/components/layout/site-document-head'
import { useAuth } from '@/contexts/auth-context'
import { useTheme } from '@/contexts/theme-context'
import { useI18n } from '@/contexts/i18n-context'
import { LocaleSwitcher } from '@/components/query/locale-switcher'
import { useSettings } from '@/contexts/settings-context'
import { QueryErrorAlert } from '@/components/results/query-error-alert'
import { ApiError } from '@/lib/api-client'
import { translateQueryError } from '@/lib/query-errors'

export function LoginPage() {
  const { login, isAuthenticated, ready } = useAuth()
  const { t } = useI18n()
  const { theme, toggleTheme } = useTheme()
  const { settings } = useSettings()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const brand = settings.header_color || '#1e3a5f'
  const hasLogo = Boolean(settings.logo_path?.trim())
  const siteName = settings.site_name || 'HopStat'

  useEffect(() => {
    if (ready && isAuthenticated) navigate('/admin', { replace: true })
  }, [ready, isAuthenticated, navigate])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      await login(email, password)
      navigate('/admin')
    } catch (err: unknown) {
      const code = err instanceof ApiError ? err.code : undefined
      const raw = err instanceof Error ? err.message : ''
      setError(translateQueryError(t, code, raw || undefined))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="admin-login-shell">
      <SiteDocumentHead pageTitle={t('login.title')} />

      <div className="admin-atmosphere" aria-hidden="true">
        <div className="admin-atmosphere__glow" />
        <div className="admin-atmosphere__grid" />
      </div>

      <div className="admin-login-toolbar">
        <LocaleSwitcher variant="admin" />
        <Button variant="outline" size="icon" className="h-8 w-8 rounded-md" onClick={toggleTheme}>
          {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
        </Button>
      </div>

      <div className="admin-login-card">
        <div className="admin-login-card__accent brand-accent-line" style={{ background: brand }} />
        <div className="admin-login-card__body">
          <div className="admin-login-card__header">
            {hasLogo ? (
              <img
                key={settings.logo_path}
                src={settings.logo_path}
                alt={siteName}
                className="admin-login-card__logo-img"
              />
            ) : (
              <div className="admin-login-card__logo" style={{ background: brand }}>
                <Network className="w-6 h-6" />
              </div>
            )}
            <h1 className="admin-login-card__title">{t('login.title')}</h1>
            <p className="admin-login-card__subtitle">{siteName}</p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            {error && <QueryErrorAlert message={error} />}
            <div className="space-y-2">
              <Label>{t('login.email')}</Label>
              <Input
                type="email"
                value={email}
                onChange={e => setEmail(e.target.value)}
                required
                autoComplete="email"
              />
            </div>
            <div className="space-y-2">
              <Label>{t('login.password')}</Label>
              <Input
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                required
                autoComplete="current-password"
              />
            </div>
            <Button type="submit" className="w-full" disabled={loading}>
              <Lock className="w-4 h-4 mr-2" />
              {t('login.submit')}
            </Button>
          </form>
        </div>
      </div>
    </div>
  )
}
