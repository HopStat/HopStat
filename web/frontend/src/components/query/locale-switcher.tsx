import { ChevronDown } from 'lucide-react'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { useI18n, type Locale } from '@/contexts/i18n-context'

interface Props {
  variant?: 'header' | 'admin'
}

const headerTriggerClass =
  'locale-switcher__select h-6 w-[3.25rem] shrink-0 gap-0.5 rounded-md border px-2 py-0 text-[11px] font-semibold tracking-wide shadow-none focus:ring-0 focus-visible:ring-0 focus-visible:ring-offset-0 [&>span]:line-clamp-none [&>svg]:h-3 [&>svg]:w-3 [&>svg]:opacity-70'

export function LocaleSwitcher({ variant = 'header' }: Props = {}) {
  const { locale, activeLocales, setLocale } = useI18n()

  if (activeLocales.length <= 1) return null

  const isAdmin = variant === 'admin'

  if (!isAdmin) {
    return (
      <div className="locale-switcher relative inline-flex items-center">
        <Select value={locale} onValueChange={value => setLocale(value as Locale)}>
          <SelectTrigger className={headerTriggerClass} aria-label="Language">
            <SelectValue />
          </SelectTrigger>
          <SelectContent
            className="locale-switcher__content min-w-[3.25rem] p-1"
            position="popper"
            align="end"
            sideOffset={4}
          >
            {activeLocales.map(code => (
              <SelectItem key={code} value={code} className="locale-switcher__item">
                {code.toUpperCase()}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    )
  }

  return (
    <div className="locale-switcher relative inline-flex items-center">
      <select
        value={locale}
        onChange={e => setLocale(e.target.value as Locale)}
        className="h-8 appearance-none cursor-pointer rounded-md border border-border bg-muted/60 pl-2.5 pr-7 text-[11px] sm:text-xs font-semibold tracking-wide text-foreground transition-colors focus:outline-none focus:ring-2 focus:ring-ring/30 hover:bg-accent"
      >
        {activeLocales.map(code => (
          <option key={code} value={code}>
            {code.toUpperCase()}
          </option>
        ))}
      </select>
      <ChevronDown className="pointer-events-none absolute right-2 h-3.5 w-3.5 text-muted-foreground" />
    </div>
  )
}
