import { ChevronDown } from 'lucide-react'
import { useI18n, type Locale } from '@/contexts/i18n-context'

interface Props {
  variant?: 'header' | 'admin'
}

export function LocaleSwitcher({ variant = 'header' }: Props = {}) {
  const { locale, activeLocales, setLocale } = useI18n()

  if (activeLocales.length <= 1) return null

  const isAdmin = variant === 'admin'

  return (
    <div className="locale-switcher relative inline-flex items-center">
      <select
        value={locale}
        onChange={e => setLocale(e.target.value as Locale)}
        className={
          isAdmin
            ? 'h-8 appearance-none cursor-pointer rounded-md border border-border bg-muted/60 pl-2.5 pr-7 text-[11px] sm:text-xs font-semibold tracking-wide text-foreground transition-colors focus:outline-none focus:ring-2 focus:ring-ring/30 hover:bg-accent'
            : 'locale-switcher__select h-6 appearance-none cursor-pointer rounded-md border pl-2 pr-6 text-[11px] font-semibold tracking-wide transition-colors focus:outline-none'
        }
      >
        {activeLocales.map(code => (
          <option key={code} value={code}>
            {code.toUpperCase()}
          </option>
        ))}
      </select>
      <ChevronDown
        className={
          isAdmin
            ? 'pointer-events-none absolute right-2 h-3.5 w-3.5 text-muted-foreground'
            : 'locale-switcher__chevron pointer-events-none absolute right-1.5 h-3 w-3'
        }
      />
    </div>
  )
}
