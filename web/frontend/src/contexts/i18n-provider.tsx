import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import i18n, {
  type Locale,
  parseActiveLanguages,
  persistLocale,
  resolveInitialLocale,
} from '@/i18n/index'
import { useSettings } from '@/contexts/settings-context'
import { I18nContext } from './i18n-context'

export function I18nProvider({ children }: { children: ReactNode }) {
  const { settings, loading: settingsLoading } = useSettings()
  const { t } = useTranslation()
  const [ready, setReady] = useState(false)

  const activeLocales = useMemo(
    () => parseActiveLanguages(settings.active_languages),
    [settings.active_languages],
  )

  const locale = (i18n.language?.split('-')[0] || 'en') as Locale

  useEffect(() => {
    if (settingsLoading) return

    const initial = resolveInitialLocale(activeLocales)
    let cancelled = false

    // No shortcut for "already on this locale": changeLanguage resolves either way, and
    // going through it uniformly keeps the ready flag out of the synchronous effect body.
    void i18n.changeLanguage(initial).then(() => {
      if (cancelled) return
      document.documentElement.lang = initial
      setReady(true)
    })

    return () => { cancelled = true }
  }, [settingsLoading, activeLocales])

  useEffect(() => {
    if (!activeLocales.includes(locale)) {
      const next = activeLocales.includes('en') ? 'en' : activeLocales[0]
      if (next) void i18n.changeLanguage(next)
    }
  }, [activeLocales, locale])

  const setLocale = useCallback((next: Locale) => {
    if (!activeLocales.includes(next)) return
    persistLocale(next)
    void i18n.changeLanguage(next)
    document.documentElement.lang = next
  }, [activeLocales])

  const value = useMemo(() => ({
    locale: activeLocales.includes(locale) ? locale : (activeLocales[0] || 'en'),
    activeLocales,
    setLocale,
    t: (key: string) => t(key),
    ready,
  }), [locale, activeLocales, setLocale, t, ready])

  if (!ready && settingsLoading) {
    return (
      <I18nContext.Provider value={{
        locale: 'en',
        activeLocales: ['en'],
        setLocale: () => {},
        t: (key: string) => key,
        ready: false,
      }}>
        {children}
      </I18nContext.Provider>
    )
  }

  return (
    <I18nContext.Provider value={value}>
      {children}
    </I18nContext.Provider>
  )
}
