import { createContext, useContext, useCallback, type ReactNode } from 'react'
import en from '@/i18n/en.json'

const translations: Record<string, string> = en as unknown as Record<string, string>

interface I18nContextValue {
  t: (key: string) => string
}

const I18nContext = createContext<I18nContextValue | null>(null)

export function I18nProvider({ children }: { children: ReactNode }) {
  const t = useCallback((key: string) => translations[key] ?? key, [])
  return (
    <I18nContext.Provider value={{ t }}>
      {children}
    </I18nContext.Provider>
  )
}

export function useI18n() {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n must be used within I18nProvider')
  return ctx
}
