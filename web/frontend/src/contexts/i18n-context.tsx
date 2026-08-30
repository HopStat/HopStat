import { createContext, useContext } from 'react'
import { type Locale, LOCALE_LABEL_KEYS } from '@/i18n/index'

export type { Locale }

export interface I18nContextValue {
  locale: Locale
  activeLocales: Locale[]
  setLocale: (locale: Locale) => void
  t: (key: string) => string
  ready: boolean
}

export const I18nContext = createContext<I18nContextValue | null>(null)

export function useI18n() {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n must be used within I18nProvider')
  return ctx
}

export function getLocaleLabelKey(code: Locale): string {
  return LOCALE_LABEL_KEYS[code]
}

