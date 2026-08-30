import { Sun, Moon } from 'lucide-react'
import { LocaleSwitcher } from '@/components/query/locale-switcher'
import { VisitorBadge } from '@/components/query/visitor-badge'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/contexts/theme-context'
import { cn } from '@/lib/utils'

interface Props {
  siteName: string
  siteDescription?: string
  logoPath?: string
  onTraceroute: (ip: string) => void
  onHomeClick: () => void
}

export function QueryHeader({ siteName, siteDescription, logoPath, onTraceroute, onHomeClick }: Props) {
  const { theme, toggleTheme } = useTheme()
  const hasLogo = Boolean(logoPath?.trim())

  const themeButton = (
    <Button
      variant="outline"
      size="icon"
      className="query-header__theme h-6 w-6 rounded-md shrink-0"
      onClick={toggleTheme}
    >
      {theme === 'dark' ? <Sun className="w-3.5 h-3.5" /> : <Moon className="w-3.5 h-3.5" />}
    </Button>
  )

  const headerToolbar = (
    <>
      <LocaleSwitcher />
      {themeButton}
    </>
  )

  return (
    <header
      className={cn('query-header relative bg-transparent', hasLogo && 'query-header--has-logo')}
      itemScope
      itemType="https://schema.org/Organization"
    >
      <div className="query-header__layout">
        <div className="query-header__brand">
          {hasLogo && (
            <button
              type="button"
              onClick={onHomeClick}
              title={siteName}
              className="query-header__logo query-header__home shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-accent-ui"
            >
              <img
                key={logoPath}
                src={logoPath}
                alt={siteName}
                itemProp="logo"
                className="h-11 w-11 sm:h-14 sm:w-14 object-contain"
              />
            </button>
          )}

          <div className="query-header__content min-w-0">
            <h1
              className="query-header__title font-display text-left truncate"
              itemProp="name"
            >
              <button type="button" onClick={onHomeClick} className="query-header__home">
                {siteName}
              </button>
            </h1>

            <div className="query-header__line-row" aria-hidden="true">
              <span className="query-header__rule-lead" />
              <div className="query-header__line-end">
                <VisitorBadge onTraceroute={onTraceroute} className="query-header__rule-ip shrink-0" />
                <div className="query-header__toolbar query-header__toolbar--desktop">
                  {headerToolbar}
                </div>
              </div>
            </div>

            {siteDescription ? (
              <p className="query-header__subtitle truncate" itemProp="description">
                <button type="button" onClick={onHomeClick} className="query-header__home">
                  {siteDescription}
                </button>
              </p>
            ) : (
              <span className="query-header__subtitle-spacer" aria-hidden="true" />
            )}
          </div>

          <div className="query-header__toolbar query-header__toolbar--mobile">
            {headerToolbar}
          </div>
        </div>

        <div className="query-header__visitor-mobile">
          <VisitorBadge onTraceroute={onTraceroute} className="max-w-full" />
        </div>
      </div>
    </header>
  )
}
