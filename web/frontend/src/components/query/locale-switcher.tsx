import { useI18n, getLocaleLabelKey } from '@/contexts/i18n-context'
import type { Locale } from '@/i18n/index'

interface Props {
  embedded?: boolean
  variant?: 'header' | 'admin'
}

export function LocaleSwitcher({ embedded, variant = 'header' }: Props = {}) {
  const { locale, activeLocales, setLocale, t } = useI18n()

  if (activeLocales.length <= 1) return null

  const shellClass =
    variant === 'admin'
      ? 'inline-flex h-8 items-stretch rounded-md border border-border bg-muted/60 overflow-hidden text-[11px] sm:text-xs font-semibold leading-none tracking-wide'
      : embedded
        ? 'inline-flex items-center gap-px text-[11px] font-semibold leading-none'
        : 'inline-flex h-6 items-stretch rounded-md border border-[color-mix(in_srgb,var(--brand-on-dark)_24%,transparent)] bg-[color-mix(in_srgb,var(--brand-on-dark)_10%,transparent)] overflow-hidden text-[11px] sm:text-xs font-semibold leading-none tracking-wide'

  return (
    <div className={shellClass}>
      {activeLocales.map(code => (
        <button
          key={code}
          type="button"
          onClick={() => setLocale(code as Locale)}
          title={t(getLocaleLabelKey(code as Locale))}
          className={
            variant === 'admin'
              ? `h-8 min-w-[1.75rem] px-2 flex items-center justify-center border-r border-border last:border-r-0 transition-colors ${
                  locale === code
                    ? 'bg-background text-foreground shadow-soft'
                    : 'text-muted-foreground hover:text-foreground hover:bg-accent'
                }`
              : embedded
                ? `px-1.5 py-1 rounded-[3px] transition-colors ${
                    locale === code
                      ? 'bg-background/90 text-foreground shadow-soft'
                      : 'text-muted-foreground hover:text-foreground'
                  }`
                : `h-6 min-w-[1.75rem] px-2 flex items-center justify-center border-r border-[color-mix(in_srgb,var(--brand-on-dark)_20%,transparent)] last:border-r-0 transition-colors ${
                    locale === code
                      ? 'bg-[color-mix(in_srgb,var(--brand-on-dark)_18%,transparent)] text-[var(--brand-on-dark)] shadow-soft'
                      : 'text-[color-mix(in_srgb,var(--brand-on-dark)_84%,transparent)] hover:text-[var(--brand-on-dark)] hover:bg-[color-mix(in_srgb,var(--brand-on-dark)_12%,transparent)]'
                  }`
          }
        >
          {code.toUpperCase()}
        </button>
      ))}
    </div>
  )
}
