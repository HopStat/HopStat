import { ChevronDown } from 'lucide-react'
import { useI18n, getLocaleLabelKey } from '@/contexts/i18n-context'
import type { Locale } from '@/i18n/index'

interface Props {
  variant?: 'header' | 'admin'
}

export function LocaleSwitcher({ variant = 'header' }: Props = {}) {
  const { locale, activeLocales, setLocale, t } = useI18n()

  if (activeLocales.length <= 1) return null

  const isAdmin = variant === 'admin'

  return (
    <div className="relative inline-flex items-center">
      <select
        value={locale}
        onChange={e => setLocale(e.target.value as Locale)}
        className={
          isAdmin
            ? 'h-8 appearance-none cursor-pointer rounded-md border border-border bg-muted/60 pl-2.5 pr-7 text-[11px] sm:text-xs font-semibold tracking-wide text-foreground transition-colors focus:outline-none focus:ring-2 focus:ring-ring/30 hover:bg-accent'
            : 'h-6 appearance-none cursor-pointer rounded-md border border-[color-mix(in_srgb,var(--brand-on-dark)_24%,transparent)] bg-[color-mix(in_srgb,var(--brand-on-dark)_10%,transparent)] pl-2 pr-6 text-[11px] font-semibold tracking-wide text-[var(--brand-on-dark)] transition-colors focus:outline-none hover:bg-[color-mix(in_srgb,var(--brand-on-dark)_18%,transparent)]'
        }
      >
        {activeLocales.map(code => (
          <option key={code} value={code}>
            {code.toUpperCase()} – {t(getLocaleLabelKey(code as Locale))}
          </option>
        ))}
      </select>
      <ChevronDown
        className={
          isAdmin
            ? 'pointer-events-none absolute right-2 h-3.5 w-3.5 text-muted-foreground'
            : 'pointer-events-none absolute right-1.5 h-3 w-3 text-[color-mix(in_srgb,var(--brand-on-dark)_70%,transparent)]'
        }
      />
    </div>
  )
}
